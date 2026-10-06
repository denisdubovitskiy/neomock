package variadic

import (
	"context"
	"errors"
	"testing"

	neomock "github.com/denisdubovitskiy/neomock"
)

// errNoRecipients — пример ошибки из Run-функции.
var errNoRecipients = errors.New("no recipients")

// TestSendExact — variadic матчивается по длине и элементам:
// When с двумя получателями ждёт ровно два.
func TestSendExact(t *testing.T) {
	m := NewMockNotifier(t)
	m.WhenSend(neomock.Context, "hi", "alice", "bob").Then(nil)

	if err := m.Send(context.Background(), "hi", "alice", "bob"); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

// TestSendEmptyVariadic — When без variadic-элементов матчится только
// на вызов с пустым списком: Send(ctx, "hi") и Send(ctx, "hi", "x") —
// разные ожидания.
func TestSendEmptyVariadic(t *testing.T) {
	m := NewMockNotifier(t)
	m.WhenSend(neomock.Context, "hi").Then(nil)

	if err := m.Send(context.Background(), "hi"); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

// TestBroadcastVoid — variadic как единственный аргумент в методе
// без результатов; Then для void-метода не принимает значений.
func TestBroadcastVoid(t *testing.T) {
	m := NewMockNotifier(t)
	m.WhenBroadcast("a", "b").Then()

	m.Broadcast("a", "b")
}

// TestCountNamedType — variadic из именованного типа: When принимает
// те же Tag, что и метод.
func TestCountNamedType(t *testing.T) {
	m := NewMockNotifier(t)
	m.WhenCount(Tag("push"), Tag("sms")).Then(2)

	if got := m.Count(Tag("push"), Tag("sms")); got != 2 {
		t.Fatalf("Count() = %d, want 2", got)
	}
}

// TestRunAnyRecipients — Run принимает variadic-сигнатуру как есть:
// удобен, когда список аргументов заранее неизвестен.
func TestRunAnyRecipients(t *testing.T) {
	m := NewMockNotifier(t)
	m.RunSend(func(_ context.Context, msg string, to ...string) error {
		if msg == "" || len(to) == 0 {
			return errNoRecipients
		}
		return nil
	})

	if err := m.Send(context.Background(), "hi", "alice"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := m.Send(context.Background(), "hi"); !errors.Is(err, errNoRecipients) {
		t.Fatalf("Send without recipients: err = %v, want %v", err, errNoRecipients)
	}
}

// TestHistory — история записывает variadic как срез (с клонированием).
func TestHistory(t *testing.T) {
	m := NewMockNotifier(t)
	m.WhenSend(neomock.Context, "hi", "alice").Then(nil)

	if err := m.Send(context.Background(), "hi", "alice"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	calls := m.SendCalls()
	if len(calls) != 1 {
		t.Fatalf("SendCalls() = %d calls, want 1", len(calls))
	}
	if len(calls[0].To) != 1 || calls[0].To[0] != "alice" {
		t.Fatalf("SendCalls()[0].To = %v, want [alice]", calls[0].To)
	}
}
