package ctx

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
)

type WebDiscoveryView struct {
	URL                string
	Origin             string
	Path               string
	StatusCode         int
	ContentLength      int64
	ContentLengthValid bool
	RedirectURL        string
	RedirectURLValid   bool
	Sources            []string
	LastSeen           string
}

func SummarizeWebDiscoveries(discoveries []WebDiscovery) []WebDiscoveryView {
	type accumulatedView struct {
		view    WebDiscoveryView
		sources map[string]struct{}
	}

	byURL := make(map[string]*accumulatedView)
	for _, discovery := range discoveries {
		item, ok := byURL[discovery.URL]
		if !ok {
			item = &accumulatedView{sources: make(map[string]struct{})}
			byURL[discovery.URL] = item
		}
		if source := strings.TrimSpace(discovery.SourceTool); source != "" {
			item.sources[source] = struct{}{}
		}
		item.view = WebDiscoveryView{
			URL:                discovery.URL,
			Origin:             webDiscoveryOrigin(discovery.URL),
			Path:               webDiscoveryPath(discovery),
			StatusCode:         discovery.StatusCode,
			ContentLength:      discovery.ContentLength,
			ContentLengthValid: discovery.ContentLengthValid,
			RedirectURL:        discovery.RedirectURL,
			RedirectURLValid:   discovery.RedirectURLValid,
			LastSeen:           discovery.UpdatedAt,
		}
	}

	views := make([]WebDiscoveryView, 0, len(byURL))
	for _, item := range byURL {
		item.view.Sources = make([]string, 0, len(item.sources))
		for source := range item.sources {
			item.view.Sources = append(item.view.Sources, source)
		}
		sort.Strings(item.view.Sources)
		views = append(views, item.view)
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].Origin != views[j].Origin {
			return views[i].Origin < views[j].Origin
		}
		if views[i].StatusCode != views[j].StatusCode {
			return views[i].StatusCode < views[j].StatusCode
		}
		if views[i].Path != views[j].Path {
			return views[i].Path < views[j].Path
		}
		return views[i].URL < views[j].URL
	})
	return views
}

func WriteWebDiscoveryList(stdout io.Writer, target *Target, discoveries []WebDiscovery) error {
	if _, err := fmt.Fprintf(stdout, "Target: %s (%s)\n", target.Name, target.IP); err != nil {
		return err
	}
	views := SummarizeWebDiscoveries(discoveries)
	if len(views) == 0 {
		_, err := fmt.Fprintln(stdout, "no web discoveries")
		return err
	}

	useColor := webColorOutputEnabled(stdout)
	for start := 0; start < len(views); {
		end := start + 1
		for end < len(views) && views[end].Origin == views[start].Origin {
			end++
		}
		if _, err := fmt.Fprintf(stdout, "\n%s\n", views[start].Origin); err != nil {
			return err
		}
		if err := writeWebDiscoveryOriginTable(stdout, views[start:end], useColor); err != nil {
			return err
		}
		start = end
	}
	return nil
}

func writeWebDiscoveryOriginTable(stdout io.Writer, views []WebDiscoveryView, useColor bool) error {
	pathWidth := len("PATH")
	lengthWidth := len("LENGTH")
	redirectWidth := len("REDIRECT")
	for _, view := range views {
		pathWidth = max(pathWidth, len(view.Path))
		lengthWidth = max(lengthWidth, len(webDiscoveryLength(view)))
		redirectWidth = max(redirectWidth, len(webDiscoveryRedirect(view)))
	}
	if _, err := fmt.Fprintf(stdout, "  STATUS  %-*s  %-*s  %-*s  SOURCES\n", pathWidth, "PATH", lengthWidth, "LENGTH", redirectWidth, "REDIRECT"); err != nil {
		return err
	}
	for _, view := range views {
		status := strconv.Itoa(view.StatusCode)
		statusPadding := strings.Repeat(" ", max(1, len("STATUS")-len(status)+2))
		if _, err := fmt.Fprintf(stdout, "  %s%s%-*s  %-*s  %-*s  %s\n",
			ColorizeHTTPStatus(view.StatusCode, useColor), statusPadding,
			pathWidth, view.Path,
			lengthWidth, webDiscoveryLength(view),
			redirectWidth, webDiscoveryRedirect(view),
			strings.Join(view.Sources, ",")); err != nil {
			return err
		}
	}
	return nil
}

func ColorizeHTTPStatus(statusCode int, enabled bool) string {
	value := strconv.Itoa(statusCode)
	if !enabled {
		return value
	}
	color := "\033[36m"
	switch {
	case statusCode >= 200 && statusCode < 300:
		color = "\033[32m"
	case statusCode >= 300 && statusCode < 400:
		color = "\033[33m"
	case statusCode >= 400 && statusCode < 500:
		color = "\033[31m"
	case statusCode >= 500:
		color = "\033[35m"
	}
	return color + value + "\033[0m"
}

func webDiscoveryOrigin(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed.Scheme + "://" + parsed.Host
	}
	return rawURL
}

func webDiscoveryPath(discovery WebDiscovery) string {
	if path := strings.TrimSpace(discovery.Path); path != "" {
		return path
	}
	parsed, err := url.Parse(discovery.URL)
	if err == nil && parsed.Path != "" {
		return parsed.Path
	}
	return "/"
}

func webDiscoveryLength(view WebDiscoveryView) string {
	if !view.ContentLengthValid {
		return "-"
	}
	return strconv.FormatInt(view.ContentLength, 10)
}

func webDiscoveryRedirect(view WebDiscoveryView) string {
	if !view.RedirectURLValid || strings.TrimSpace(view.RedirectURL) == "" {
		return "-"
	}
	return view.RedirectURL
}

func webColorOutputEnabled(writer io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
