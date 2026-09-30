//go:build system

package m4_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/felixgeelhaar/glossa/platform/internal/cli"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// The visual layer, for real (RFC 0005 §12.2).
//
// `glossa capture --check` drives the headless Chrome this test
// started over the repository's preview build, in German and in
// Japanese, at 1280×800. The probe pass measures in the live page —
// `scrollWidth` against `clientWidth` on the region's clipping host —
// and the checkout page's pay button, 104 px wide and single-line,
// holds eight full-width Japanese glyphs where the German has twelve
// Latin ones. Nothing is stubbed: no browser is faked, no finding is
// injected, and the manifest the server stores is the one the page
// produced.

// captureJSON is `glossa capture --json`, with `--check`'s document in
// it.
type captureJSON struct {
	Captures []struct {
		Route    string `json:"route"`
		Locale   string `json:"locale"`
		Viewport struct {
			Width, Height int
		} `json:"viewport"`
		Regions int `json:"regions"`
		Visible int `json:"visible"`
		Probes  int `json:"probes"`
		Image   struct {
			SHA256        string `json:"sha256"`
			Width, Height int
		} `json:"image"`
	} `json:"captures"`
	Upload *struct {
		Build              string   `json:"build"`
		Captures           int      `json:"captures"`
		ImagesStored       int      `json:"images_stored"`
		ImagesDeduplicated int      `json:"images_deduplicated"`
		UnknownKeys        []string `json:"unknown_keys"`
		Findings           int      `json:"findings"`
	} `json:"upload"`
	Check *checkJSON `json:"check"`
}

// cropResult is what part 2 read back out of the stored screenshot.
type cropResult struct {
	Capture        string
	Region         string
	Key, Locale    string
	Route          string
	Box            box
	ImageBytes     int
	ImageW, ImageH int
	Scale          float64
	CropW, CropH   int
	// Colours is how many distinct colours the crop holds: a crop of a
	// button with text in it is not one flat rectangle.
	Colours int
	OK      bool
	Why     string
}

type box struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type messageCaptures struct {
	Key      string `json:"key"`
	Captures []struct {
		ID       string `json:"id"`
		Route    string `json:"route"`
		Locale   string `json:"locale"`
		Viewport struct {
			Width, Height int
		} `json:"viewport"`
		Image struct {
			Digest        string `json:"digest"`
			Width, Height int
			URL           string `json:"url"`
		} `json:"image"`
		Regions []struct {
			Kind    string `json:"kind"`
			Visible bool   `json:"visible"`
			Box     box    `json:"box"`
		} `json:"regions"`
	} `json:"captures"`
}

// captureAndCheck runs the workflow's capture step twice against the
// preview build. Twice, because the two-sighting rule (§5.2) makes a
// visual finding evidence only when the same fingerprint comes back in
// the next capture of the same route, viewport and locale — which is
// what §12.4's policy v4 then needs in order to fail a build.
func (s *scenario) captureAndCheck() {
	t := s.t
	app := serveApp(t, filepath.Join(s.repo.dir, "built", "preview"))
	s.appURL = app.URL
	s.ci.env["GLOSSA_CAPTURE_CDP"] = s.chrome

	var out captureJSON
	uploaded := 0
	for sighting, commit := range []string{headCommit, headCommit2} {
		res := s.ci.run("capture", "--check", "--upload", "--no-coverage",
			"--base-url", app.URL, "--commit", commit, "--branch", prBranch, "--json")
		if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
			s.gap("12.2", "`glossa capture --check` printed no document (exit %d): %s", res.code, res.stderr)
			return
		}
		// The upload is idempotent by the manifest's own hash, so a
		// second identical capture replays and stores nothing. The
		// findings the server took are the first run's.
		if out.Upload != nil && out.Upload.Findings > uploaded {
			uploaded = out.Upload.Findings
		}
		if res.code != int(cli.ExitCheckFailed) {
			s.note("12.2", "`glossa capture --check` (sighting %d, `%s`) exited %d.", sighting+1, short(commit), res.code)
		}
	}
	s.cliCapture = out
	s.uploadedFindings = uploaded

	want := 2 // one route × two locales × one viewport
	if len(out.Captures) != want {
		s.gap("12.2", "the capture took %d screenshots, want %d (/kasse × de, ja × 1280×800)", len(out.Captures), want)
	}
	if out.Upload == nil {
		s.gap("12.2", "the capture uploaded nothing")
		return
	}
	if out.Check == nil {
		s.gap("12.2", "`glossa capture --check` produced no check document")
		return
	}
	clipped := visualFindings(out.Check.Findings, "text-clipped")
	ja, de := 0, 0
	for _, f := range clipped {
		if f.Locus.Key != keyButton {
			s.gap("12.2", "a `text-clipped` finding names `%s`, not the pay button `%s`: the fixture's checkout "+
				"page moved and the stylesheet is measuring the wrong line", f.Locus.Key, keyButton)
			continue
		}
		switch f.Locus.Locale {
		case "ja":
			ja++
		case "de":
			de++
		}
	}
	switch {
	case ja == 0:
		s.gap("12.2", "the Japanese checkout button did not clip: %d text-clipped findings in all (%d German). "+
			"The probe measured %d regions over %d captures.",
			len(clipped), de, totalRegions(out), len(out.Captures))
	default:
		s.note("12.2", "The visual layer is real: `glossa capture --check` drove Chrome over `/kasse` in German "+
			"and Japanese at 1280×800, and the Japanese pay button clipped — %d `text-clipped` finding(s) on the "+
			"Japanese capture (%d on the German one), and the server stored %d probe findings with the upload.",
			ja, de, uploaded)
	}
}

func totalRegions(out captureJSON) int {
	n := 0
	for _, c := range out.Captures {
		n += c.Regions
	}
	return n
}

func visualFindings(fs []domain.Finding, code string) []domain.Finding {
	var out []domain.Finding
	for _, f := range fs {
		if f.Layer == domain.LayerVisual && f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

// cropRegion reads the clipped region back through the API and crops
// the stored screenshot to it — the evidence Studio shows, taken the
// way Studio takes it.
func (s *scenario) cropRegion() {
	if s.cliCapture.Check == nil {
		s.crop.Why = "there was no capture check to read a region from"
		return
	}
	// The finding to crop is the server's, not the CLI's. A probe
	// finding is minted in the page, where there is no capture yet —
	// the CLI's own copy carries `locus.region` and an empty
	// `locus.capture` — and the capture id is filled in at the ingest,
	// which is where a finding becomes something Studio can point at an
	// image.
	var finding *domain.Finding
	stored := queryFindings(s.owner, s.projectPath("/findings"), url.Values{"layer": {string(domain.LayerVisual)}})
	for i, f := range stored {
		if f.Code == "text-clipped" && f.Locus.Locale == "ja" && f.Locus.Key == keyButton {
			finding = &stored[i]
			break
		}
	}
	if finding == nil {
		s.crop.Why = fmt.Sprintf("the server stored %d visual findings, so there is no region to read back", len(stored))
		s.gap("12.2", "the region the finding names cannot be read back and cropped: the server stored %d "+
			"visual findings, and %d probe findings were reported as uploaded. `glossa capture --upload` "+
			"carries the probe pass's findings on the manifest (`cli/capture.Findings`) and the ingest "+
			"records them (`context/app.recordFindings`), so the break is in one of the three links "+
			"between them: the manifest's `findings`, the key the ingest resolved, or the policy's "+
			"grading of the visual layer",
			len(stored), s.uploadedFindings)
		return
	}
	s.crop.Key, s.crop.Locale = finding.Locus.Key, finding.Locus.Locale
	s.crop.Capture, s.crop.Region = finding.Locus.Capture, finding.Locus.Region
	if finding.Locus.Capture == "" || finding.Locus.Region == "" {
		s.gap("12.2", "the visual finding names no capture and region, so nothing can be cropped: locus %+v", finding.Locus)
		s.crop.Why = "the finding carries no capture or region"
		return
	}

	// Where the region is: the message's captures, the one the finding
	// names.
	// The branch matters: the endpoint answers with the captures of the
	// *current* builds, and this capture was taken by the pull
	// request's CI.
	var mc messageCaptures
	s.owner.do(http.MethodGet, s.projectPath("/messages/"+finding.Locus.Key+"/captures?branch="+url.QueryEscape(prBranch)),
		nil, http.StatusOK, &mc)
	var ids []string
	for _, c := range mc.Captures {
		ids = append(ids, short(c.ID)+" "+c.Locale)
		if c.ID != finding.Locus.Capture {
			continue
		}
		if len(c.Regions) == 0 {
			break
		}
		s.crop.Box = c.Regions[0].Box
		s.crop.Route = c.Route
		s.crop.ImageW, s.crop.ImageH = c.Image.Width, c.Image.Height
		if c.Viewport.Width > 0 {
			s.crop.Scale = float64(c.Image.Width) / float64(c.Viewport.Width)
		}
	}
	if s.crop.Box.Width == 0 {
		s.gap("12.2", "the API gave no region box for capture %s of `%s`; `…/messages/%s/captures?branch=%s` "+
			"answered with %d captures (%s)", short(finding.Locus.Capture), finding.Locus.Key,
			finding.Locus.Key, prBranch, len(mc.Captures), strings.Join(ids, ", "))
		s.crop.Why = "no region box"
		return
	}

	raw, headers, err := s.owner.raw(s.projectPath("/captures/" + finding.Locus.Capture + "/image"))
	if err != nil {
		s.gap("12.2", "the capture's image could not be read: %v", err)
		s.crop.Why = err.Error()
		return
	}
	s.crop.ImageBytes = len(raw)
	if ct := headers.Get("Content-Type"); ct != "image/png" {
		s.gap("12.2", "the capture's image came back as %q, want image/png", ct)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		s.gap("12.2", "the stored screenshot is not a PNG: %v", err)
		s.crop.Why = err.Error()
		return
	}
	scale := s.crop.Scale
	if scale <= 0 {
		scale = 1
	}
	r := image.Rect(
		int(float64(s.crop.Box.X)*scale), int(float64(s.crop.Box.Y)*scale),
		int(float64(s.crop.Box.X+s.crop.Box.Width)*scale), int(float64(s.crop.Box.Y+s.crop.Box.Height)*scale),
	).Intersect(img.Bounds())
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok || r.Empty() {
		s.gap("12.2", "the region %v does not land inside the %v screenshot", r, img.Bounds())
		s.crop.Why = "the region is outside the image"
		return
	}
	crop := sub.SubImage(r)
	s.crop.CropW, s.crop.CropH = crop.Bounds().Dx(), crop.Bounds().Dy()
	s.crop.Colours = colours(crop)
	// A crop of a button with text in it is not one flat rectangle. If
	// it were, the box would be pointing at nothing.
	if s.crop.Colours < 2 {
		s.gap("12.2", "the crop of %s is one flat colour: the region does not point at the button", finding.Locus.Region)
		s.crop.Why = "the crop is flat"
		return
	}
	s.crop.OK = true
	s.note("12.2", "The region the finding names (`%s` on capture `%s`) was read back through the API and the "+
		"stored screenshot cropped to it: %d×%d CSS px at (%d, %d) → %d×%d pixels of a %d×%d screenshot, %d "+
		"distinct colours.", finding.Locus.Region, short(finding.Locus.Capture),
		s.crop.Box.Width, s.crop.Box.Height, s.crop.Box.X, s.crop.Box.Y,
		s.crop.CropW, s.crop.CropH, s.crop.ImageW, s.crop.ImageH, s.crop.Colours)
}

// colours counts the distinct colours in an image, up to a bound.
func colours(img image.Image) int {
	seen := map[uint64]bool{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y && len(seen) < 4096; y++ {
		for x := b.Min.X; x < b.Max.X && len(seen) < 4096; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			seen[uint64(r)<<48|uint64(g)<<32|uint64(bl)<<16|uint64(a)] = true
		}
	}
	return len(seen)
}

func (b box) String() string { return fmt.Sprintf("%d×%d at (%d, %d)", b.Width, b.Height, b.X, b.Y) }
