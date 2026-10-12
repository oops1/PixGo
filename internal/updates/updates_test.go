package updates

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const feedTwoReleases = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Release notes from PixGo</title>
  <entry>
    <id>tag:github.com,2008:Repository/1414714282/v0.3.0-rc.1</id>
    <link rel="alternate" type="text/html" href="https://github.com/oops1/PixGo/releases/tag/v0.3.0-rc.1"/>
    <title>PixGo 0.3.0-rc.1</title>
  </entry>
  <entry>
    <id>tag:github.com,2008:Repository/1414714282/v0.2.1</id>
    <link rel="alternate" type="text/html" href="https://github.com/oops1/PixGo/releases/tag/v0.2.1"/>
    <title>PixGo 0.2.1</title>
  </entry>
  <entry>
    <id>tag:github.com,2008:Repository/1414714282/v0.2.0</id>
    <link rel="alternate" type="text/html" href="https://github.com/oops1/PixGo/releases/tag/v0.2.0"/>
    <title>PixGo 0.2.0</title>
  </entry>
</feed>`

type fakeGitHub struct {
	feedStatus int
	feedBody   string
	apiStatus  int
	apiBody    string
	apiHeaders map[string]string
	feedHits   atomic.Int32
	apiHits    atomic.Int32
	agent      atomic.Value
	accept     atomic.Value
}

func (f *fakeGitHub) serve(t *testing.T) *Source {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/feed", func(w http.ResponseWriter, r *http.Request) {
		f.feedHits.Add(1)
		f.agent.Store(r.Header.Get("User-Agent"))
		f.accept.Store(r.Header.Get("Accept"))
		w.WriteHeader(f.feedStatus)
		_, _ = w.Write([]byte(f.feedBody))
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		f.apiHits.Add(1)
		for k, v := range f.apiHeaders {
			w.Header().Set(k, v)
		}
		w.WriteHeader(f.apiStatus)
		_, _ = w.Write([]byte(f.apiBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Source{Client: srv.Client(), UserAgent: "PixGo/test", FeedURL: srv.URL + "/feed", APIURL: srv.URL + "/api"}
}

func TestLatestTakesTheFirstStableEntryOfTheFeed(t *testing.T) {
	f := &fakeGitHub{feedStatus: 200, feedBody: feedTwoReleases}
	src := f.serve(t)

	got, err := src.Latest(t.Context())

	if err != nil {
		t.Fatal(err)
	}
	if got.Tag != "v0.2.1" || got.Version() != "0.2.1" {
		t.Fatalf("release = %+v, want the stable v0.2.1 and not the prerelease", got)
	}
	if got.Page != "https://github.com/oops1/PixGo/releases/tag/v0.2.1" {
		t.Fatalf("page = %q", got.Page)
	}
	if f.apiHits.Load() != 0 {
		t.Fatal("the API must not be touched while the feed works")
	}
}

func TestLatestIdentifiesItselfToGitHub(t *testing.T) {
	f := &fakeGitHub{feedStatus: 200, feedBody: feedTwoReleases}
	src := f.serve(t)

	if _, err := src.Latest(t.Context()); err != nil {
		t.Fatal(err)
	}

	if f.agent.Load() != "PixGo/test" {
		t.Fatalf("user agent = %v", f.agent.Load())
	}
	if f.accept.Load() != "application/atom+xml" {
		t.Fatalf("accept = %v", f.accept.Load())
	}
}

func TestLatestFallsBackToTheAPIWhenTheFeedFails(t *testing.T) {
	f := &fakeGitHub{
		feedStatus: 500,
		apiStatus:  200,
		apiBody:    `{"tag_name":"v0.4.0","html_url":"https://github.com/oops1/PixGo/releases/tag/v0.4.0","prerelease":false}`,
	}
	src := f.serve(t)

	got, err := src.Latest(t.Context())

	if err != nil {
		t.Fatal(err)
	}
	if got.Tag != "v0.4.0" || f.feedHits.Load() != 1 || f.apiHits.Load() != 1 {
		t.Fatalf("release = %+v, feed hits %d, api hits %d", got, f.feedHits.Load(), f.apiHits.Load())
	}
}

func TestLatestFallsBackWhenTheFeedHasNoStableRelease(t *testing.T) {
	feed := strings.Replace(feedTwoReleases, "v0.2.1", "v0.2.1-beta", 2)
	feed = strings.Replace(feed, "v0.2.0", "v0.2.0-beta", 2)
	f := &fakeGitHub{feedStatus: 200, feedBody: feed, apiStatus: 200, apiBody: `{"tag_name":"v0.1.9"}`}
	src := f.serve(t)

	got, err := src.Latest(t.Context())

	if err != nil || got.Tag != "v0.1.9" {
		t.Fatalf("release = %+v, err = %v", got, err)
	}
}

func TestLatestReportsTheRateLimitOfTheAPI(t *testing.T) {
	f := &fakeGitHub{
		feedStatus: 404,
		apiStatus:  403,
		apiHeaders: map[string]string{"X-RateLimit-Remaining": "0"},
	}
	src := f.serve(t)

	_, err := src.Latest(t.Context())

	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func TestLatestReportsTheFeedErrorWhenBothFail(t *testing.T) {
	f := &fakeGitHub{feedStatus: 502, apiStatus: 500}
	src := f.serve(t)

	_, err := src.Latest(t.Context())

	if !errors.Is(err, ErrStatus) || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v, want the feed status", err)
	}
}

func TestLatestRejectsAForbiddenAnswerWithoutAQuota(t *testing.T) {
	f := &fakeGitHub{feedStatus: 404, apiStatus: 403}
	src := f.serve(t)

	_, err := src.Latest(t.Context())

	if errors.Is(err, ErrRateLimited) || !errors.Is(err, ErrStatus) {
		t.Fatalf("err = %v, want a plain status error", err)
	}
}

func TestLatestSkipsPrereleasesAndDraftsOfTheAPI(t *testing.T) {
	for _, body := range []string{
		`{"tag_name":"v0.5.0","prerelease":true}`,
		`{"tag_name":"v0.5.0","draft":true}`,
		`{"tag_name":""}`,
		`{"tag_name":"nightly"}`,
	} {
		f := &fakeGitHub{feedStatus: 500, apiStatus: 200, apiBody: body}
		src := f.serve(t)

		_, err := src.Latest(t.Context())

		if err == nil {
			t.Errorf("%s: want an error", body)
		}
	}
}

func TestLatestFailsOnBrokenFeedAndBrokenAPI(t *testing.T) {
	f := &fakeGitHub{feedStatus: 200, feedBody: "<feed", apiStatus: 200, apiBody: "{"}
	src := f.serve(t)

	if _, err := src.Latest(t.Context()); err == nil {
		t.Fatal("want an error")
	}
}

func TestLatestFailsOnAnEmptyFeed(t *testing.T) {
	f := &fakeGitHub{feedStatus: 200, feedBody: `<feed xmlns="http://www.w3.org/2005/Atom"></feed>`, apiStatus: 500}
	src := f.serve(t)

	_, err := src.Latest(t.Context())

	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestLatestStopsWhenTheContextIsCancelled(t *testing.T) {
	f := &fakeGitHub{feedStatus: 200, feedBody: feedTwoReleases}
	src := f.serve(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := src.Latest(ctx); err == nil {
		t.Fatal("a cancelled lookup must fail")
	}
}

func TestLatestFailsOnAnUnusableURL(t *testing.T) {
	src := &Source{Client: http.DefaultClient, FeedURL: "http://bad host/", APIURL: "http://bad host/"}

	if _, err := src.Latest(t.Context()); err == nil {
		t.Fatal("want an error")
	}
}

func TestLatestReadsOnlyTheBodyLimit(t *testing.T) {
	filler := strings.Repeat("x", bodyLimit)
	f := &fakeGitHub{feedStatus: 200, feedBody: "<feed>" + filler + feedTwoReleases, apiStatus: 500}
	src := f.serve(t)

	if _, err := src.Latest(t.Context()); err == nil {
		t.Fatal("a body past the limit must not be accepted")
	}
}

func TestNewSourceUsesTheProjectFeedAndATimeout(t *testing.T) {
	src := NewSource("PixGo/1.0")

	if src.FeedURL != FeedURL || src.APIURL != APIURL || src.UserAgent != "PixGo/1.0" {
		t.Fatalf("source = %+v", src)
	}
	if src.Client.Timeout != Timeout {
		t.Fatalf("timeout = %v", src.Client.Timeout)
	}
}

func TestReleasePageMustStayInsideTheProjectOnGitHub(t *testing.T) {
	cases := map[string]string{
		"https://github.com/oops1/PixGo/releases/tag/v1.0.0":    "https://github.com/oops1/PixGo/releases/tag/v1.0.0",
		"http://github.com/oops1/PixGo/releases/tag/v1.0.0":     PageURL,
		"https://evil.example.com/oops1/PixGo/releases/tag/v1":  PageURL,
		"https://github.com/someone/else/releases/tag/v1":       PageURL,
		"https://github.com.evil.example/oops1/PixGo/releases/": PageURL,
		"file:///etc/passwd": PageURL,
		"":                   PageURL,
		"::bad::":            PageURL,
	}
	for in, want := range cases {
		if got := safePage(in); got != want {
			t.Errorf("safePage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTagIsTheLastSegmentOfTheReleaseLink(t *testing.T) {
	cases := map[string]string{
		"https://github.com/oops1/PixGo/releases/tag/v0.2.0":    "v0.2.0",
		"https://github.com/oops1/PixGo/releases/tag/v0.3.0-rc": "v0.3.0-rc",
		"https://github.com/oops1/PixGo/releases":               "",
		"https://github.com/oops1/PixGo/pull/4":                 "",
		"::bad::":                                               "",
	}
	for in, want := range cases {
		if got := tagOfPage(in); got != want {
			t.Errorf("tagOfPage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewerComparesNumbersNotText(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.2.0", "v0.2.1", true},
		{"0.2.1", "v0.2.1", false},
		{"0.2.1", "0.2.0", false},
		{"0.9.0", "0.10.0", true},
		{"0.10.0", "0.9.0", false},
		{"1.0", "1.0.1", true},
		{"1.0.0", "1.0", false},
		{"v0.2.0", "0.2.0", false},
		{"0.2.0", "1.0.0", true},
		{"0.2.1-ci.3", "0.2.1", true},
		{"0.2.1", "0.2.1-ci.3", false},
		{"0.2.1-ci.3", "0.2.1-ci.10", true},
		{"0.2.1-ci.10", "0.2.1-ci.3", false},
		{"0.2.1-ci.3", "0.2.1-ci.3", false},
		{"0.2.1-ci", "0.2.1-ci.1", true},
		{"0.2.1-1", "0.2.1-alpha", true},
		{"0.2.1-alpha", "0.2.1-1", false},
		{"0.2.1-alpha", "0.2.1-beta", true},
		{"0.2.0+build.5", "0.2.0", false},
		{"dev", "0.2.0", true},
		{"dev+abc1234", "0.2.0", true},
		{"0.2.0", "", false},
		{"0.2.0", "nightly", false},
		{"0.2.0", "1.x", false},
	}
	for _, c := range cases {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestComparableTellsReleaseBuildsFromDevelopmentBuilds(t *testing.T) {
	for in, want := range map[string]bool{
		"0.2.1": true, "v0.2.1": true, "0.2.1-ci.3": true,
		"dev": false, "dev+abc1234": false, "": false, "1.x": false, "-1": false,
	} {
		if got := Comparable(in); got != want {
			t.Errorf("Comparable(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestStableRejectsPrereleasesAndNonVersions(t *testing.T) {
	for in, want := range map[string]bool{
		"v0.2.0": true, "0.2.0+meta": true, "v0.2.0-rc.1": false, "nightly": false, "": false,
	} {
		if got := Stable(in); got != want {
			t.Errorf("Stable(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDueWaitsForTheIntervalAndSurvivesClockJumps(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		last time.Time
		want bool
	}{
		{"never checked", time.Time{}, true},
		{"checked a minute ago", now.Add(-time.Minute), false},
		{"checked just inside the interval", now.Add(-Interval + time.Second), false},
		{"checked exactly one interval ago", now.Add(-Interval), true},
		{"checked long ago", now.Add(-30 * 24 * time.Hour), true},
		{"last check is in the future", now.Add(time.Hour), true},
	}
	for _, c := range cases {
		if got := Due(c.last, now); got != c.want {
			t.Errorf("%s: Due = %v, want %v", c.name, got, c.want)
		}
	}
}
