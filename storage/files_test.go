package storage

import (
	"strings"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"photo.jpg":            "photo.jpg",
		"../../etc/passwd":     "passwd",
		`C:\temp\file.txt`:     "file.txt",
		"fi<le>?:*.png":        "fi_le____.png",
		"CON.txt":              "_CON.txt",
		"nul":                  "_nul",
		"  spaced.txt  ":       "spaced.txt",
		"file.txt.":            "file.txt",
		"нормальный файл.jpg":  "нормальный файл.jpg",
		"bad\x00name\x01.text": "badname.text",
	}

	for input, want := range cases {
		got, err := SanitizeName(input)
		if err != nil {
			t.Errorf("SanitizeName(%q) error: %v", input, err)
			continue
		}

		if got != want {
			t.Errorf("SanitizeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSanitizeNameRejectsEmpty(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", "   ", ".", "..", "/"} {
		if _, err := SanitizeName(input); err == nil {
			t.Errorf("SanitizeName(%q) должна возвращать ошибку", input)
		}
	}
}

func TestSanitizeNameTruncatesLongNames(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("я", 400) + ".jpg"

	got, err := SanitizeName(long)
	if err != nil {
		t.Fatalf("SanitizeName() error: %v", err)
	}

	if len(got) > maxNameLength {
		t.Errorf("длина имени %d превышает лимит %d", len(got), maxNameLength)
	}

	if !strings.HasSuffix(got, ".jpg") {
		t.Errorf("расширение потерялось: %q", got)
	}
}

func TestSaveStreamCreatesUniqueNames(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())

	first, err := store.SaveStream("photo.jpg", strings.NewReader("one"))
	if err != nil {
		t.Fatalf("SaveStream() error: %v", err)
	}

	second, err := store.SaveStream("photo.jpg", strings.NewReader("two"))
	if err != nil {
		t.Fatalf("SaveStream() error: %v", err)
	}

	if first == second {
		t.Fatalf("имена должны различаться: %q", first)
	}

	if second != "photo (1).jpg" {
		t.Errorf("second = %q, want %q", second, "photo (1).jpg")
	}
}

func TestSaveStreamIntoNestedPath(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())

	name, err := store.SaveStream("../../evil.txt", strings.NewReader("data"))
	if err != nil {
		t.Fatalf("SaveStream() error: %v", err)
	}

	if name != "evil.txt" {
		t.Errorf("name = %q, want %q", name, "evil.txt")
	}
}

func TestSaveStreamRequiresBaseDir(t *testing.T) {
	t.Parallel()

	store := New("")

	if _, err := store.SaveStream("a.txt", strings.NewReader("x")); err == nil {
		t.Fatal("ожидалась ошибка при пустой папке хранилища")
	}
}
