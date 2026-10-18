package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBackupWritesConsistentCopy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO students(full_name, role) VALUES ('Петренко Іван', 'student')`); err != nil {
		t.Fatalf("seed student: %v", err)
	}

	dest := filepath.Join(dir, "backups", "copy.db")
	if err := Backup(ctx, db, dest); err != nil {
		t.Fatalf("Backup() error: %v", err)
	}
	if err := Backup(ctx, db, dest); err == nil {
		t.Fatal("Backup() overwrote an existing file")
	}

	copyDB, err := Open(ctx, dest)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	t.Cleanup(func() { _ = copyDB.Close() })
	var count int
	if err := copyDB.QueryRow(`SELECT COUNT(*) FROM students`).Scan(&count); err != nil {
		t.Fatalf("count students in backup: %v", err)
	}
	if count != 1 {
		t.Fatalf("backup has %d students, want 1", count)
	}
}

func TestImportNamesSkipsBlankAndDuplicateNames(t *testing.T) {
	ctx := context.Background()
	db := repositoryTestDB(t)
	students := NewStudentRepository(db)

	added, err := students.ImportNames(ctx, []string{"Петренко Іван", "", "  Шевченко   Олена ", "Петренко Іван"})
	if err != nil {
		t.Fatalf("ImportNames() error: %v", err)
	}
	if added != 2 {
		t.Fatalf("added = %d, want 2", added)
	}
	added, err = students.ImportNames(ctx, []string{"Шевченко Олена"})
	if err != nil {
		t.Fatalf("second ImportNames() error: %v", err)
	}
	if added != 0 {
		t.Fatalf("re-import added %d, want 0", added)
	}
	list, err := students.ListUnregistered(ctx)
	if err != nil {
		t.Fatalf("list students: %v", err)
	}
	if len(list) != 2 || list[1].FullName != "Шевченко Олена" {
		t.Fatalf("unexpected roster: %+v", list)
	}
}
