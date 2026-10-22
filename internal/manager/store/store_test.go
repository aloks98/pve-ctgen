package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestCorruptStoreIsPreserved(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "nodes.yaml")
	bad := []byte("[invalid YAML")
	if err = os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = db.CreateNode("node", "", "localhost:1", "test"); err == nil {
		t.Fatal("corrupt data overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(bad) {
		t.Fatal("existing data modified")
	}
}
func TestConcurrentStoresAndPermissions(t *testing.T) {
	dir := t.TempDir()
	a, _ := Open(dir)
	b, _ := Open(dir)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db := a
			if i%2 == 1 {
				db = b
			}
			if _, err := db.CreateNode(fmt.Sprintf("node-%d", i), "", "localhost:1", "secret"); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	nodes, err := a.ListNodes()
	if err != nil || len(nodes) != 20 {
		t.Fatalf("lost updates: count=%d err=%v", len(nodes), err)
	}
	st, _ := os.Stat(filepath.Join(dir, "nodes.yaml"))
	if st.Mode().Perm() != 0600 {
		t.Fatalf("credentials permission %o", st.Mode().Perm())
	}
	if err = a.UpdateBuildStatus("missing", "cancelled"); err == nil {
		t.Fatal("phantom build created")
	}
}
