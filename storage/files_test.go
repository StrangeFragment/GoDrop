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

func TestListReturnsNewestFirst(t *testing.T) {
	t.Parallel()

	store := New(t.TempDir())

	for _, name := range []string{"first.jpg", "second.jpg", "third.jpg"} {
		if _, err := store.SaveStream(name, strings.NewReader("data")); err != nil {
			t.Fatalf("SaveStream(%q) error: %v", name, err)
		}
	}

	files, err := store.List(0)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("получено %d файлов, want 3", len(files))
	}

	for _, file := range files {
		if file.Size != 4 {
			t.Errorf("размер %q = %d, want 4", file.Name, file.Size)
		}

		if !strings.Contains(file.RelPath, "/") {
			t.Errorf("RelPath %q должен содержать папку", file.RelPath)
		}
	}

	limited, err := store.List(2)
	if err != nil {
		t.Fatalf("List(2) error: %v", err)
	}

	if len(limited) != 2 {
		t.Errorf("List(2) вернул %d файлов, want 2", len(limited))
	}
}

func TestResolveRejectsTraversal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := New(dir)

	if _, err := store.SaveStream("ok.txt", strings.NewReader("data")); err != nil {
		t.Fatalf("SaveStream() error: %v", err)
	}

	files, err := store.List(1)
	if err != nil || len(files) != 1 {
		t.Fatalf("List() = %v, %v", files, err)
	}

	if _, err := store.Resolve(files[0].RelPath); err != nil {
		t.Errorf("Resolve(%q) error: %v", files[0].RelPath, err)
	}

	bad := []string{
		"",
		"../secret.txt",
		"2026-01-01/../../secret.txt",
		`..\secret.txt`,
		"/etc/passwd",
		"..",
		"2026-01-01/",
	}

	for _, path := range bad {
		if _, err := store.Resolve(path); err == nil {
			t.Errorf("Resolve(%q) должна возвращать ошибку", path)
		}
	}
}
