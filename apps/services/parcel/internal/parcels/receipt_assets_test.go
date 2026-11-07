package parcels

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReceiptFontsAreParsedOnce(t *testing.T) {
	first, err := loadReceiptFonts()
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadReceiptFonts()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("the receipt font set was rebuilt instead of cached")
	}
	if len(first.faces) != len(receiptFontFiles) {
		t.Fatalf("faces = %d, want %d", len(first.faces), len(receiptFontFiles))
	}
	if len(first.latin) == 0 || len(first.latinZlib) == 0 {
		t.Fatal("the Latin font resources were not prepared")
	}
	if first.byName[receiptLatinFont] == nil {
		t.Fatal("the Latin face is missing from the cache")
	}
}

func TestReceiptFontInitializationFailureIsReported(t *testing.T) {
	if _, err := parseReceiptFonts(receiptFontFiles, func(string) ([]byte, error) {
		return nil, errors.New("missing asset")
	}); err == nil || !strings.Contains(err.Error(), "read embedded receipt font") {
		t.Fatalf("missing-font error = %v", err)
	}
	if _, err := parseReceiptFonts(receiptFontFiles, func(string) ([]byte, error) {
		return []byte("not a font"), nil
	}); err == nil || !strings.Contains(err.Error(), "parse embedded receipt font") {
		t.Fatalf("invalid-font error = %v", err)
	}
}

func TestReceiptConcurrentGenerationUsesSharedFontCache(t *testing.T) {
	parcel := Detail{
		TrackingNumber: "SDKCONCURRENT",
		Status:         "CREATED",
		Sender:         Contact{Name: "නිමල් පෙරේරා", City: "Colombo"},
		Recipient:      Contact{Name: "Amali Fernando", City: "Kandy"},
		WeightKG:       200,
		SizeCategory:   "SMALL",
		Fee:            Fee{"LKR", 45000, 10000, 0, 55000},
		CreatedAt:      time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC),
	}
	const workers = 8
	const perWorker = 4
	var wg sync.WaitGroup
	failures := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				pdf, err := Receipt(parcel)
				if err != nil {
					failures <- err
					return
				}
				if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) || !bytes.Contains(pdf, []byte("/ToUnicode")) {
					failures <- errors.New("concurrent receipt was not a Unicode PDF")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
}

func TestReceiptLinesBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		lines int
	}{
		{"39 characters", strings.Repeat("a", 39), 1},
		{"40 characters", strings.Repeat("a", 40), 1},
		{"41 characters", strings.Repeat("a", 41), 2},
		{"long unicode", strings.Repeat("ක", 81), 3},
		{"40-character first word", strings.Repeat("a", 40) + " next", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines := receiptLines(tc.input)
			if len(lines) != tc.lines {
				t.Fatalf("lines = %d, want %d: %q", len(lines), tc.lines, lines)
			}
			for _, line := range lines {
				if line == "" {
					t.Fatalf("blank receipt line in %q", lines)
				}
				if len([]rune(line)) > 40 {
					t.Fatalf("line %q exceeds 40 runes", line)
				}
			}
			if got, want := strings.Join(strings.Fields(strings.Join(lines, " ")), ""), strings.Join(strings.Fields(tc.input), ""); got != want {
				t.Fatalf("wrapping lost text: %q != %q", got, want)
			}
		})
	}
}
