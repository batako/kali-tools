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
	ID                 int64
	DiscoveryType      string
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
	TemplateURL        string
	ParameterName      string
	ParameterValue     string
	FuzzPart           string
	WordCount          int
	WordCountValid     bool
	LineCount          int
	LineCountValid     bool
	Wordlist           string
	CommandLogID       int64
	CommandLogIDValid  bool
}

func SummarizeWebDiscoveries(discoveries []WebDiscovery) []WebDiscoveryView {
	type accumulatedView struct {
		view    WebDiscoveryView
		sources map[string]struct{}
	}

	byURL := make(map[string]*accumulatedView)
	for _, discovery := range discoveries {
		key := webDiscoveryType(discovery.DiscoveryType) + "\x00" + discovery.URL
		item, ok := byURL[key]
		if !ok {
			item = &accumulatedView{sources: make(map[string]struct{})}
			byURL[key] = item
		}
		if source := strings.TrimSpace(discovery.SourceTool); source != "" {
			item.sources[source] = struct{}{}
		}
		item.view = WebDiscoveryView{
			ID:                 discovery.ID,
			DiscoveryType:      webDiscoveryType(discovery.DiscoveryType),
			URL:                discovery.URL,
			Origin:             webDiscoveryOrigin(discovery.URL),
			Path:               webDiscoveryPath(discovery),
			StatusCode:         discovery.StatusCode,
			ContentLength:      discovery.ContentLength,
			ContentLengthValid: discovery.ContentLengthValid,
			RedirectURL:        discovery.RedirectURL,
			RedirectURLValid:   discovery.RedirectURLValid,
			LastSeen:           discovery.UpdatedAt,
			TemplateURL:        discovery.TemplateURL,
			ParameterName:      discovery.ParameterName,
			ParameterValue:     discovery.ParameterValue,
			FuzzPart:           discovery.FuzzPart,
			WordCount:          discovery.WordCount,
			WordCountValid:     discovery.WordCountValid,
			LineCount:          discovery.LineCount,
			LineCountValid:     discovery.LineCountValid,
			Wordlist:           discovery.Wordlist,
			CommandLogID:       discovery.CommandLogID,
			CommandLogIDValid:  discovery.CommandLogIDValid,
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
		if err := writeWebDiscoveryOriginTables(stdout, views[start:end], useColor); err != nil {
			return err
		}
		start = end
	}
	return nil
}

func writeWebDiscoveryOriginTables(stdout io.Writer, views []WebDiscoveryView, useColor bool) error {
	sections := []struct {
		typeName string
		title    string
	}{
		{typeName: "path", title: "Paths"},
		{typeName: "param-name", title: "Parameter names"},
		{typeName: "param-value", title: "Parameter values"},
	}
	for _, section := range sections {
		var selected []WebDiscoveryView
		for _, view := range views {
			if view.DiscoveryType == section.typeName {
				selected = append(selected, view)
			}
		}
		if len(selected) == 0 {
			continue
		}
		if _, err := fmt.Fprintln(stdout, section.title); err != nil {
			return err
		}
		if section.typeName == "path" {
			if err := writeWebDiscoveryOriginTable(stdout, selected, useColor); err != nil {
				return err
			}
		} else if err := writeWebParameterTable(stdout, selected, useColor); err != nil {
			return err
		}
	}
	return nil
}

func writeWebParameterTable(stdout io.Writer, views []WebDiscoveryView, useColor bool) error {
	pathWidth, parameterWidth, valueWidth := len("PATH"), len("PARAMETER"), len("VALUE")
	lengthWidth, redirectWidth := len("LENGTH"), len("REDIRECT")
	for _, view := range views {
		pathWidth = max(pathWidth, len(view.Path))
		parameterWidth = max(parameterWidth, len(view.ParameterName))
		valueWidth = max(valueWidth, len(view.ParameterValue))
		lengthWidth = max(lengthWidth, len(webDiscoveryLength(view)))
		redirectWidth = max(redirectWidth, len(webDiscoveryRedirect(view)))
	}
	if _, err := fmt.Fprintf(stdout, "  ID  STATUS  %-*s  %-*s  %-*s  %-*s  %-*s  SOURCES\n", pathWidth, "PATH", parameterWidth, "PARAMETER", valueWidth, "VALUE", lengthWidth, "LENGTH", redirectWidth, "REDIRECT"); err != nil {
		return err
	}
	for _, view := range views {
		if _, err := fmt.Fprintf(stdout, "  %-2d  %-6s  %-*s  %-*s  %-*s  %-*s  %-*s  %s\n", view.ID, ColorizeHTTPStatus(view.StatusCode, useColor), pathWidth, view.Path, parameterWidth, view.ParameterName, valueWidth, view.ParameterValue, lengthWidth, webDiscoveryLength(view), redirectWidth, webDiscoveryRedirect(view), strings.Join(view.Sources, ",")); err != nil {
			return err
		}
	}
	return nil
}

func WriteWebDiscoveryDetail(stdout io.Writer, target *Target, discovery *WebDiscovery) error {
	values := [][2]string{
		{"ID", strconv.FormatInt(discovery.ID, 10)},
		{"Type", webDiscoveryType(discovery.DiscoveryType)},
		{"Target", fmt.Sprintf("%s (%s)", target.Name, target.IP)},
		{"URL", discovery.URL},
		{"Template", discovery.TemplateURL},
		{"Path", discovery.Path},
		{"Parameter", discovery.ParameterName},
		{"Value", discovery.ParameterValue},
		{"Fuzz part", discovery.FuzzPart},
		{"Status", strconv.Itoa(discovery.StatusCode)},
		{"Length", optionalInt64(discovery.ContentLength, discovery.ContentLengthValid)},
		{"Words", optionalInt(discovery.WordCount, discovery.WordCountValid)},
		{"Lines", optionalInt(discovery.LineCount, discovery.LineCountValid)},
		{"Redirect", optionalString(discovery.RedirectURL, discovery.RedirectURLValid)},
		{"Source", discovery.SourceTool},
		{"Wordlist", discovery.Wordlist},
		{"Observed", discovery.UpdatedAt},
	}
	if discovery.CommandLogIDValid {
		values = append(values, [2]string{"Command log", strconv.FormatInt(discovery.CommandLogID, 10)})
	}
	for _, item := range values {
		if item[1] == "" {
			continue
		}
		if _, err := fmt.Fprintf(stdout, "%-12s %s\n", item[0]+":", item[1]); err != nil {
			return err
		}
	}
	return nil
}

func optionalInt(value int, valid bool) string {
	if !valid {
		return "-"
	}
	return strconv.Itoa(value)
}

func optionalInt64(value int64, valid bool) string {
	if !valid {
		return "-"
	}
	return strconv.FormatInt(value, 10)
}

func optionalString(value string, valid bool) string {
	if !valid {
		return "-"
	}
	return value
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
