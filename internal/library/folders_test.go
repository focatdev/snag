package library

import (
	"testing"

	"snag/internal/ffmpeg"
)

func TestFolders(t *testing.T) {
	dir := t.TempDir()
	l := Open(dir, func() ffmpeg.Tools { return ffmpeg.Tools{} }, nil)
	a := l.Add(NewClip{Path: dir + `\a.mp4`, Game: "Roblox"})
	b := l.Add(NewClip{Path: dir + `\b.mp4`, Game: "Roblox"})

	f, err := l.CreateFolder("  best   of  ")
	if err != nil || f.Name != "best of" {
		t.Fatalf("create: %+v %v", f, err)
	}
	if _, err := l.CreateFolder("Best Of"); err == nil {
		t.Fatal("duplicate name should fail")
	}
	if _, err := l.CreateFolder("   "); err == nil {
		t.Fatal("empty name should fail")
	}
	other, _ := l.CreateFolder("another")
	if _, err := l.RenameFolder(other.ID, "BEST OF"); err == nil {
		t.Fatal("rename onto a taken name should fail")
	}
	if _, err := l.RenameFolder(f.ID, "Best of"); err != nil {
		t.Fatalf("renaming to a new case of the same name: %v", err)
	}

	id := f.ID
	if err := l.UpdateMany([]string{a.ID, b.ID}, Patch{Folder: &id}); err != nil {
		t.Fatal(err)
	}
	gone := "nope"
	if err := l.UpdateMany([]string{a.ID}, Patch{Folder: &gone}); err == nil {
		t.Fatal("moving into a missing folder should fail")
	}
	if c, _ := l.Get(a.ID); c.Folder != id {
		t.Fatalf("clip a folder = %q", c.Folder)
	}

	reopened := Open(dir, func() ffmpeg.Tools { return ffmpeg.Tools{} }, nil)
	if got := reopened.Folders(); len(got) != 2 || got[0].Name != "another" || got[1].Name != "Best of" {
		t.Fatalf("folders after reopen: %+v", got)
	}

	if err := l.DeleteFolder(id); err != nil {
		t.Fatal(err)
	}
	if c, _ := l.Get(b.ID); c.Folder != "" {
		t.Fatalf("clip b still in deleted folder %q", c.Folder)
	}
	if len(l.Folders()) != 1 {
		t.Fatalf("folders after delete: %+v", l.Folders())
	}
}
