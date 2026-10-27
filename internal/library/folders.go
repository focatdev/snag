package library

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"snag/internal/config"
)

type Folder struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// own file so older versions can still read library.json
func (l *Library) loadFolders() {
	b, err := os.ReadFile(l.foldersPath)
	if err != nil {
		return
	}
	var list []Folder
	if json.Unmarshal(b, &list) == nil {
		l.folders = list
	}
}

func (l *Library) saveFoldersLocked() error {
	b, err := json.MarshalIndent(l.folders, "", " ")
	if err != nil {
		return err
	}
	return config.WriteFileAtomic(l.foldersPath, b)
}

func (l *Library) Folders() []Folder {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := append([]Folder{}, l.folders...)
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func (l *Library) folderIndexLocked(id string) int {
	for i, f := range l.folders {
		if f.ID == id {
			return i
		}
	}
	return -1
}

func (l *Library) checkNameLocked(name, except string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	switch {
	case name == "":
		return "", errors.New("give the folder a name")
	case utf8.RuneCountInString(name) > 60:
		return "", errors.New("keep the name under 60 characters")
	}
	for _, f := range l.folders {
		if f.ID != except && strings.EqualFold(f.Name, name) {
			return "", errors.New("you already have a folder called " + f.Name)
		}
	}
	return name, nil
}

func (l *Library) CreateFolder(name string) (Folder, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	name, err := l.checkNameLocked(name, "")
	if err != nil {
		return Folder{}, err
	}
	f := Folder{ID: newID(), Name: name, CreatedAt: time.Now()}
	l.folders = append(l.folders, f)
	if err := l.saveFoldersLocked(); err != nil {
		l.folders = l.folders[:len(l.folders)-1]
		return Folder{}, err
	}
	return f, nil
}

func (l *Library) RenameFolder(id, name string) (Folder, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	i := l.folderIndexLocked(id)
	if i < 0 {
		return Folder{}, errors.New("folder not found")
	}
	name, err := l.checkNameLocked(name, id)
	if err != nil {
		return Folder{}, err
	}
	old := l.folders[i].Name
	l.folders[i].Name = name
	if err := l.saveFoldersLocked(); err != nil {
		l.folders[i].Name = old
		return Folder{}, err
	}
	return l.folders[i], nil
}

// the clips stay in the library, they just leave the folder
func (l *Library) DeleteFolder(id string) error {
	l.mu.Lock()
	i := l.folderIndexLocked(id)
	if i < 0 {
		l.mu.Unlock()
		return errors.New("folder not found")
	}
	l.folders = append(l.folders[:i], l.folders[i+1:]...)
	err := l.saveFoldersLocked()
	for _, c := range l.clips {
		if c.Folder == id {
			c.Folder = ""
		}
	}
	l.mu.Unlock()
	l.changed()
	return err
}
