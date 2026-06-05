package service

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"

	"github.com/hieu-seta/seta-training/pkg/httpx"
	"golang.org/x/sync/errgroup"
)

// WorkerCount fixed per stage_2 spec — don't tune w/o profile.
const WorkerCount = 10

// expectedHeader: order strict, case-insensitive.
var expectedHeader = []string{"username", "email", "password", "role"}

// RowError reports a single row that failed Register.
type RowError struct {
	Line  int    `json:"line"`
	Email string `json:"email,omitempty"`
	Err   string `json:"error"`
}

// Summary returned by ImportService.Run.
type Summary struct {
	Processed int        `json:"processed"`
	Failed    []RowError `json:"failed"`
}

// ImportService wraps AuthService.Register w/ a worker pool.
type ImportService struct {
	auth *AuthService
}

// NewImport builds an ImportService.
func NewImport(a *AuthService) *ImportService { return &ImportService{auth: a} }

type row struct {
	line     int
	username string
	email    string
	password string
	role     string
}

// Run reads csv from r, validates header, dispatches rows to 10 workers, collects failures.
// Returns Summary on success (incl. partial). Fatal errors (bad header, I/O) → wrapped httpx.ErrBadRequest / ErrInternal.
func (s *ImportService) Run(ctx context.Context, r io.Reader) (*Summary, error) {
	rd := csv.NewReader(r)
	rd.FieldsPerRecord = 4
	rd.TrimLeadingSpace = true

	header, err := rd.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: csv header: %w", httpx.ErrBadRequest, err)
	}
	if !validHeader(header) {
		return nil, fmt.Errorf("%w: csv header must be: %s", httpx.ErrBadRequest, strings.Join(expectedHeader, ","))
	}

	g, gctx := errgroup.WithContext(ctx)
	rows := make(chan row, 100)
	fails := make(chan RowError, 1024)
	var total atomic.Int32

	// Producer
	g.Go(func() error {
		defer close(rows)
		line := 1 // header was line 1
		for {
			rec, err := rd.Read()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("%w: csv read: %w", httpx.ErrBadRequest, err)
			}
			line++
			total.Add(1)
			r := row{
				line:     line,
				username: strings.TrimSpace(rec[0]),
				email:    strings.TrimSpace(rec[1]),
				password: rec[2],
				role:     strings.TrimSpace(rec[3]),
			}
			select {
			case rows <- r:
			case <-gctx.Done():
				return gctx.Err()
			}
		}
	})

	// Workers
	for i := 0; i < WorkerCount; i++ {
		g.Go(func() error {
			for r := range rows {
				if _, err := s.auth.Register(gctx, r.username, r.email, r.password, r.role); err != nil {
					select {
					case fails <- RowError{Line: r.line, Email: r.email, Err: err.Error()}:
					case <-gctx.Done():
						return gctx.Err()
					}
				}
			}
			return nil
		})
	}

	// Close fails channel once all workers + producer finish.
	go func() {
		_ = g.Wait()
		close(fails)
	}()

	// Drain fails (main goroutine).
	var failed []RowError
	for fe := range fails {
		failed = append(failed, fe)
	}

	if err := g.Wait(); err != nil {
		// Producer error (bad header / I/O) — preserve sentinel.
		return nil, err
	}

	return &Summary{
		Processed: int(total.Load()) - len(failed),
		Failed:    failed,
	}, nil
}

func validHeader(h []string) bool {
	if len(h) != len(expectedHeader) {
		return false
	}
	for i, want := range expectedHeader {
		if !strings.EqualFold(strings.TrimSpace(h[i]), want) {
			return false
		}
	}
	return true
}
