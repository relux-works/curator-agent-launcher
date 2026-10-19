package fragment

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	managerschema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

// The independent wire reader must agree with the original reader on every
// existing fragment corpus file and protocol vector, including rejected JSON.
// Schema-invalid documents can still be syntactically valid CCJ-1 JSON here.
func TestProtocolReaderAlignment(t *testing.T) {
	compare := func(t *testing.T, data []byte) {
		t.Helper()
		v, oldErr := ParseJSON(data)
		got, newErr := managerschema.CanonicalJSON(data)
		if (oldErr == nil) != (newErr == nil) {
			t.Fatalf("reader verdict differs: original=%v protocol=%v", oldErr, newErr)
		}
		if oldErr == nil && !bytes.Equal(Canonical(v), got) {
			t.Fatalf("canonical bytes differ: original=%q protocol=%q", Canonical(v), got)
		}
	}
	err := filepath.WalkDir("testdata", func(name string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(name, ".json") {
			return err
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		t.Run(name, func(t *testing.T) { compare(t, data) })
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	files, err := managerschema.Documents.ReadDir("vectors")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		data, err := managerschema.Documents.ReadFile("vectors/" + file.Name())
		if err != nil {
			t.Fatal(err)
		}
		var vectors []struct {
			Name     string          `json:"name"`
			Document json.RawMessage `json:"document"`
		}
		if err := json.Unmarshal(data, &vectors); err != nil {
			t.Fatal(err)
		}
		for _, vector := range vectors {
			t.Run(file.Name()+"/"+vector.Name, func(t *testing.T) { compare(t, vector.Document) })
		}
	}
	// Corpus documents are predominantly valid JSON. These neighboring values
	// explicitly require both acceptance and rejection at strict-reader bounds.
	for i, data := range [][]byte{
		[]byte(`{"z":1,"a":"é"}`), []byte(`{"a":1,"a":2}`), []byte(`"\ud800"`),
		{'"', 0xff, '"'}, []byte(`9007199254740991`), []byte(`9007199254740992`),
		[]byte(`-0`), []byte(`1.5`), []byte(`1e2`), []byte(`{} {}`),
	} {
		t.Run(string(rune('a'+i)), func(t *testing.T) { compare(t, data) })
	}
}
