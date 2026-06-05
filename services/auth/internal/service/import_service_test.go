package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hieu-seta/seta-training/pkg/httpx"
	"github.com/hieu-seta/seta-training/pkg/jwtauth"
	"github.com/hieu-seta/seta-training/services/auth/internal/service"
)

func newImportSvc(t *testing.T) (*service.ImportService, *fakeUserRepo) {
	t.Helper()
	u := newFakeUserRepo()
	r := newFakeRefreshRepo()
	auth := service.New(u, r, jwtauth.NewSigner("import_test_secret_at_least_32_____x"), service.Config{
		BcryptCost: 4,
	})
	return service.NewImport(auth), u
}

const okCSV = `username,email,password,role
alice,alice@example.com,password123,manager
bob,bob@example.com,password123,member
carol,carol@example.com,password123,member
`

func TestImport_Happy(t *testing.T) {
	s, u := newImportSvc(t)
	sum, err := s.Run(context.Background(), strings.NewReader(okCSV))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if sum.Processed != 3 || len(sum.Failed) != 0 {
		t.Errorf("got %+v", sum)
	}
	list, _ := u.List(context.Background(), 100, 0)
	if len(list) != 3 {
		t.Errorf("expected 3 in repo, got %d", len(list))
	}
}

func TestImport_BadHeader_BadRequest(t *testing.T) {
	csv := "name,email,pw,role\nx,x@x.x,p,member\n"
	s, _ := newImportSvc(t)
	_, err := s.Run(context.Background(), strings.NewReader(csv))
	if !errors.Is(err, httpx.ErrBadRequest) {
		t.Errorf("want bad req, got %v", err)
	}
}

func TestImport_MalformedRow_AbortsAsBadRequest(t *testing.T) {
	// Row has 3 fields instead of 4 → csv.Reader returns ErrFieldCount → producer returns ErrBadRequest.
	csv := "username,email,password,role\nalice,alice@example.com,password123\n"
	s, _ := newImportSvc(t)
	_, err := s.Run(context.Background(), strings.NewReader(csv))
	if !errors.Is(err, httpx.ErrBadRequest) {
		t.Errorf("want bad req, got %v", err)
	}
}

func TestImport_DuplicateRows_AppearInFailed(t *testing.T) {
	csv := "username,email,password,role\n" +
		"alice,dup@example.com,password123,member\n" +
		"alice2,dup@example.com,password123,member\n" +
		"bob,bob@example.com,password123,member\n"
	s, _ := newImportSvc(t)
	sum, err := s.Run(context.Background(), strings.NewReader(csv))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if sum.Processed != 2 || len(sum.Failed) != 1 {
		t.Errorf("got processed=%d failed=%d (%+v)", sum.Processed, len(sum.Failed), sum.Failed)
	}
	// Order non-deterministic w/ 10 workers — either row 2 or row 3 loses the race.
	if sum.Failed[0].Email != "dup@example.com" {
		t.Errorf("failed row email: %+v", sum.Failed[0])
	}
	if sum.Failed[0].Line != 2 && sum.Failed[0].Line != 3 {
		t.Errorf("failed row line want 2 or 3, got %d", sum.Failed[0].Line)
	}
}

func TestImport_InvalidRoleRow_AppearsInFailed(t *testing.T) {
	csv := "username,email,password,role\n" +
		"alice,alice@example.com,password123,manager\n" +
		"bob,bob@example.com,password123,admin\n" // invalid role
	s, _ := newImportSvc(t)
	sum, err := s.Run(context.Background(), strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Processed != 1 || len(sum.Failed) != 1 {
		t.Errorf("got %+v", sum)
	}
}

func TestImport_Large1k(t *testing.T) {
	var b strings.Builder
	b.WriteString("username,email,password,role\n")
	for i := 0; i < 1000; i++ {
		b.WriteString("user,user")
		b.WriteString(itoa(i))
		b.WriteString("@example.com,password123,member\n")
	}
	s, _ := newImportSvc(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sum, err := s.Run(ctx, strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if sum.Processed != 1000 || len(sum.Failed) != 0 {
		t.Errorf("got processed=%d failed=%d", sum.Processed, len(sum.Failed))
	}
}

func TestImport_ContextCancel(t *testing.T) {
	var b strings.Builder
	b.WriteString("username,email,password,role\n")
	for i := 0; i < 5000; i++ {
		b.WriteString("u,u")
		b.WriteString(itoa(i))
		b.WriteString("@e.f,password123,member\n")
	}
	s, _ := newImportSvc(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before start
	_, err := s.Run(ctx, strings.NewReader(b.String()))
	if err == nil {
		t.Error("expected ctx cancellation error")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [12]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
