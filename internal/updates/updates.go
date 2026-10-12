package updates

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	FeedURL  = "https://github.com/oops1/PixGo/releases.atom"
	APIURL   = "https://api.github.com/repos/oops1/PixGo/releases/latest"
	PageURL  = "https://github.com/oops1/PixGo/releases/latest"
	Interval = 72 * time.Hour
	Timeout  = 15 * time.Second

	bodyLimit = 1 << 20
	pageHost  = "github.com"
)

var (
	ErrUnavailable = errors.New("updates: the release feed has no stable release")
	ErrStatus      = errors.New("updates: the release feed answered with an error")
	ErrRateLimited = errors.New("updates: github answered that the request quota is spent")
)

type Release struct {
	Tag  string
	Page string
}

func (r Release) Version() string {
	return strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(r.Tag), "v"), "V")
}

type Source struct {
	Client    *http.Client
	UserAgent string
	FeedURL   string
	APIURL    string
}

func NewSource(userAgent string) *Source {
	return &Source{
		Client:    &http.Client{Timeout: Timeout},
		UserAgent: userAgent,
		FeedURL:   FeedURL,
		APIURL:    APIURL,
	}
}

func (s *Source) Latest(ctx context.Context) (Release, error) {
	release, feedErr := s.fromFeed(ctx)
	if feedErr == nil {
		return release, nil
	}
	release, apiErr := s.fromAPI(ctx)
	if apiErr == nil {
		return release, nil
	}
	if errors.Is(apiErr, ErrRateLimited) {
		return Release{}, apiErr
	}
	return Release{}, feedErr
}

type atomFeed struct {
	Entries []struct {
		Links []struct {
			Href string `xml:"href,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

func (s *Source) fromFeed(ctx context.Context) (Release, error) {
	var feed atomFeed
	err := s.get(ctx, s.FeedURL, "application/atom+xml", func(body io.Reader) error {
		return xml.NewDecoder(body).Decode(&feed)
	})
	if err != nil {
		return Release{}, err
	}
	for _, entry := range feed.Entries {
		for _, link := range entry.Links {
			tag := tagOfPage(link.Href)
			if tag == "" || !Stable(tag) {
				continue
			}
			return Release{Tag: tag, Page: safePage(link.Href)}, nil
		}
	}
	return Release{}, ErrUnavailable
}

type apiRelease struct {
	Tag        string `json:"tag_name"`
	Page       string `json:"html_url"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

func (s *Source) fromAPI(ctx context.Context) (Release, error) {
	var info apiRelease
	err := s.get(ctx, s.APIURL, "application/vnd.github+json", func(body io.Reader) error {
		return json.NewDecoder(body).Decode(&info)
	})
	if err != nil {
		return Release{}, err
	}
	tag := strings.TrimSpace(info.Tag)
	if tag == "" || info.Prerelease || info.Draft || !Stable(tag) {
		return Release{}, ErrUnavailable
	}
	return Release{Tag: tag, Page: safePage(info.Page)}, nil
}

func (s *Source) get(ctx context.Context, rawURL, accept string, decode func(io.Reader) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", s.UserAgent)
	resp, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError(resp)
	}
	return decode(io.LimitReader(resp.Body, bodyLimit))
}

func statusError(resp *http.Response) error {
	limited := resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests
	if limited && (resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != "") {
		return ErrRateLimited
	}
	return fmt.Errorf("%w: %s", ErrStatus, resp.Status)
}

func tagOfPage(href string) string {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil || !strings.Contains(u.Path, "/releases/tag/") {
		return ""
	}
	return path.Base(u.Path)
}

func safePage(href string) string {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil || u.Scheme != "https" || u.Host != pageHost || !strings.HasPrefix(u.Path, "/oops1/PixGo/") {
		return PageURL
	}
	return u.String()
}

func Due(last, now time.Time) bool {
	return last.IsZero() || now.Before(last) || !now.Before(last.Add(Interval))
}

type version struct {
	numbers []int
	pre     []string
}

func parse(text string) (version, bool) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(strings.TrimPrefix(text, "v"), "V")
	if cut := strings.IndexByte(text, '+'); cut >= 0 {
		text = text[:cut]
	}
	var v version
	if cut := strings.IndexByte(text, '-'); cut >= 0 {
		v.pre = strings.Split(text[cut+1:], ".")
		text = text[:cut]
	}
	for _, part := range strings.Split(text, ".") {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return version{}, false
		}
		v.numbers = append(v.numbers, n)
	}
	return v, true
}

func Comparable(text string) bool {
	_, ok := parse(text)
	return ok
}

func Stable(text string) bool {
	v, ok := parse(text)
	return ok && len(v.pre) == 0
}

func Newer(current, latest string) bool {
	want, ok := parse(latest)
	if !ok {
		return false
	}
	have, _ := parse(current)
	return compare(want, have) > 0
}

func compare(a, b version) int {
	for i := 0; i < len(a.numbers) || i < len(b.numbers); i++ {
		if x, y := numberAt(a.numbers, i), numberAt(b.numbers, i); x != y {
			return sign(x - y)
		}
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := compareIdentifier(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return sign(len(a.pre) - len(b.pre))
}

func compareIdentifier(a, b string) int {
	x, errA := strconv.Atoi(a)
	y, errB := strconv.Atoi(b)
	switch {
	case errA == nil && errB == nil:
		return sign(x - y)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func numberAt(numbers []int, i int) int {
	if i < len(numbers) {
		return numbers[i]
	}
	return 0
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
