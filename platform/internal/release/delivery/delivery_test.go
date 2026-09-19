package delivery_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/objectstore"
	"github.com/felixgeelhaar/glossa/platform/internal/release/delivery"
)

const project = "0192f5a0-7a4e-7cc3-9d1e-3a4b5c6d7e8f"

func TestNewKey(t *testing.T) {
	a, err := delivery.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := delivery.NewKey()
	if a == b || !delivery.ValidKey(a) || !strings.HasPrefix(a, "glossa_pk_") || len(a) != len("glossa_pk_")+32 {
		t.Fatalf("keys %q %q", a, b)
	}
	for _, bad := range []string{"", "glossa_api_" + strings.Repeat("a", 43), "glossa_pk_short", a + "x", "glossa_pk_" + strings.Repeat("/", 32)} {
		if delivery.ValidKey(bad) {
			t.Errorf("ValidKey(%q) = true", bad)
		}
	}
}

func TestEnvironmentNames(t *testing.T) {
	for _, ok := range []string{"production", "preview-42", "dev", "a1", "0"} {
		if !delivery.ValidEnvironment(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "a", "Prod", "-x", "x_y", strings.Repeat("x", 64)} {
		if delivery.ValidEnvironment(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestLayoutKeysAreValidObjectKeys(t *testing.T) {
	key, _ := delivery.NewKey()
	digest := delivery.Digest([]byte("x"))
	for _, k := range []string{
		delivery.KeyIndexPath(key),
		delivery.ManifestPath(project, "production"),
		delivery.ArtifactPath(project, digest),
	} {
		if err := objectstore.CheckKey(k); err != nil {
			t.Errorf("%s: %v", k, err)
		}
	}
	if strings.Contains(delivery.KeyIndexPath(key), key) {
		t.Error("the key itself appears in its index path")
	}
	if got := delivery.ArtifactPath(project, digest); got != "v1/projects/"+project+"/a/"+digest+".json" {
		t.Errorf("artifact path %s", got)
	}
}

func TestKeyIndexRoundTrip(t *testing.T) {
	b := delivery.EncodeKeyIndex(project, "k1")
	got, err := delivery.DecodeKeyIndex(b)
	if err != nil || got.Project != project || got.KeyID != "k1" {
		t.Fatalf("%+v, %v", got, err)
	}
	for _, bad := range []string{`{}`, `{"schema":"glossa.delivery-key/v2","project":"` + project + `"}`,
		`{"schema":"glossa.delivery-key/v1","project":"../x"}`, `nope`} {
		if _, err := delivery.DecodeKeyIndex([]byte(bad)); !errors.Is(err, delivery.ErrInvalidKeyIndex) {
			t.Errorf("DecodeKeyIndex(%s) = %v", bad, err)
		}
	}
}

func TestDigest(t *testing.T) {
	if got := delivery.Digest([]byte("abc")); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" || !delivery.ValidDigest(got) {
		t.Fatalf("digest %s", got)
	}
	if delivery.ValidDigest("BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD") {
		t.Error("uppercase digest accepted")
	}
}
