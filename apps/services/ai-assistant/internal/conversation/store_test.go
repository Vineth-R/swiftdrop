package conversation

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type sequentialReader struct{ next byte }

func (r *sequentialReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.next
		r.next++
	}
	return len(p), nil
}

func fixedIDReader() *sequentialReader {
	return &sequentialReader{}
}

func TestCreateAndLoad(t *testing.T) {
	store := New(10, time.Minute, 10, WithRandom(fixedIDReader()))
	item, err := store.Create("user-1", false)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !ValidID(item.ID) {
		t.Fatalf("ID %q is not valid", item.ID)
	}
	if item.Anonymous || item.OwnerID != "user-1" {
		t.Fatalf("unexpected ownership: %+v", item)
	}

	loaded, err := store.Load(item.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != item.ID || len(loaded.Messages) != 0 {
		t.Fatalf("unexpected loaded conversation: %+v", loaded)
	}
}

func TestCreateRejectsInvalidOwnership(t *testing.T) {
	store := New(10, time.Minute, 10, WithRandom(fixedIDReader()))
	if _, err := store.Create("user-1", true); err == nil {
		t.Fatal("expected anonymous owner error")
	}
	if _, err := store.Create("  ", false); err == nil {
		t.Fatal("expected missing owner error")
	}
}

func TestLoadRejectsInvalidAndUnknown(t *testing.T) {
	store := New(10, time.Minute, 10, WithRandom(fixedIDReader()))
	if _, err := store.Load("nope"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("err = %v, want ErrInvalidID", err)
	}
	if _, err := store.Load("cv00000000000000000000000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestExpiry(t *testing.T) {
	now := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	store := New(10, time.Minute, 10, WithRandom(fixedIDReader()), WithClock(func() time.Time { return now }))
	item, err := store.Create("", true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	now = now.Add(2 * time.Minute)
	if _, err := store.Load(item.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound after expiry", err)
	}
}

func TestAppendTrimsHistory(t *testing.T) {
	store := New(10, time.Minute, 3, WithRandom(fixedIDReader()))
	item, err := store.Create("", true)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, text := range []string{"one", "two", "three", "four"} {
		if err := store.Append(item.ID, Message{Role: RoleUser, Text: text}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	loaded, err := store.Load(item.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Messages) != 3 {
		t.Fatalf("len(Messages) = %d, want 3", len(loaded.Messages))
	}
	if loaded.Messages[0].Text != "two" || loaded.Messages[2].Text != "four" {
		t.Fatalf("unexpected trimmed history: %+v", loaded.Messages)
	}
}

func TestEvictsOldestWhenFull(t *testing.T) {
	store := New(2, time.Hour, 10, WithRandom(fixedIDReader()))
	first, _ := store.Create("", true)
	second, _ := store.Create("", true)
	third, _ := store.Create("", true)

	if _, err := store.Load(first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("first conversation should be evicted, err = %v", err)
	}
	if _, err := store.Load(second.ID); err != nil {
		t.Fatalf("second conversation: %v", err)
	}
	if _, err := store.Load(third.ID); err != nil {
		t.Fatalf("third conversation: %v", err)
	}
}

func TestEvictsOldestWhenTimestampsAreEqual(t *testing.T) {
	frozen := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	store := New(2, time.Hour, 10,
		WithRandom(fixedIDReader()),
		WithClock(func() time.Time { return frozen }),
	)
	first, _ := store.Create("", true)
	second, _ := store.Create("", true)
	third, _ := store.Create("", true)

	if _, err := store.Load(first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("first conversation should be evicted, err = %v", err)
	}
	if _, err := store.Load(second.ID); err != nil {
		t.Fatalf("second conversation: %v", err)
	}
	if _, err := store.Load(third.ID); err != nil {
		t.Fatalf("third conversation: %v", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	store := New(500, time.Hour, 10)
	const workers = 40
	var wait sync.WaitGroup
	wait.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wait.Done()
			item, err := store.Create("", true)
			if err != nil {
				t.Errorf("Create: %v", err)
				return
			}
			if err := store.Append(item.ID, Message{Role: RoleUser, Text: "hello"}); err != nil {
				t.Errorf("Append: %v", err)
				return
			}
			if _, err := store.Load(item.ID); err != nil {
				t.Errorf("Load: %v", err)
			}
		}()
	}
	wait.Wait()
}

func TestSnapshotsDoNotAliasStore(t *testing.T) {
	store := New(10, time.Minute, 10, WithRandom(fixedIDReader()))
	item, _ := store.Create("", true)
	if err := store.Append(item.ID, Message{Role: RoleUser, Text: "hello"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	loaded, _ := store.Load(item.ID)
	loaded.Messages[0].Text = "mutated"

	again, _ := store.Load(item.ID)
	if again.Messages[0].Text != "hello" {
		t.Fatal("store was mutated through a snapshot")
	}
}
