package xwebshell

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var Version = "dev"

const webshellRoot = "/usr/share/webshells"

type catalogEntry struct {
	Category    string
	Name        string
	Description string
	Path        string
	Group       bool
}

type listedEntry struct {
	catalogEntry
	status string
	id     int
}

var catalog = []catalogEntry{
	{Category: "ASP", Name: "cmd-asp-5.1", Description: "ASP command shell", Path: "asp/cmd-asp-5.1.asp"},
	{Category: "ASP", Name: "cmdasp", Description: "ASP command shell", Path: "asp/cmdasp.asp"},
	{Category: "ASPX", Name: "cmdasp", Description: "ASP.NET command shell", Path: "aspx/cmdasp.aspx"},
	{Category: "CFM", Name: "cfexec", Description: "ColdFusion command shell", Path: "cfm/cfexec.cfm"},
	{Category: "JSP", Name: "cmdjsp", Description: "JSP command shell", Path: "jsp/cmdjsp.jsp"},
	{Category: "JSP", Name: "jsp-reverse", Description: "JSP reverse shell", Path: "jsp/jsp-reverse.jsp"},
	{Category: "Perl", Name: "perl-reverse-shell", Description: "Perl reverse shell", Path: "perl/perl-reverse-shell.pl"},
	{Category: "Perl", Name: "perlcmd", Description: "Perl command shell", Path: "perl/perlcmd.cgi"},
	{Category: "PHP", Name: "findsocket", Description: "PHP socket shell with a helper source", Path: "php/findsocket", Group: true},
	{Category: "PHP", Name: "php-backdoor", Description: "PHP backdoor", Path: "php/php-backdoor.php"},
	{Category: "PHP", Name: "php-reverse-shell", Description: "PHP reverse shell", Path: "php/php-reverse-shell.php"},
	{Category: "PHP", Name: "qsd-php-backdoor", Description: "PHP backdoor", Path: "php/qsd-php-backdoor.php"},
	{Category: "PHP", Name: "simple-backdoor", Description: "Simple PHP backdoor", Path: "php/simple-backdoor.php"},
	{Category: "Laudanum", Name: "Laudanum", Description: "Multi-language Laudanum shell collection", Path: "laudanum", Group: true},
	{Category: "SecLists / CFM", Name: "shell.cfm", Description: "ColdFusion shell collection", Path: "seclists/CFM", Group: true},
	{Category: "SecLists / FuzzDB", Name: "FuzzDB web shells", Description: "FuzzDB command, upload, and reverse shell collection", Path: "seclists/FuzzDB", Group: true},
	{Category: "SecLists / JSP", Name: "simple-shell.jsp", Description: "JSP shell collection", Path: "seclists/JSP", Group: true},
	{Category: "SecLists / Magento", Name: "Magento shells", Description: "Magento administration shell collection", Path: "seclists/Magento", Group: true},
	{Category: "SecLists / PHP", Name: "PHP shells", Description: "PHP shell collection", Path: "seclists/PHP", Group: true},
	{Category: "SecLists / WordPress", Name: "WordPress shells", Description: "WordPress shell collection", Path: "seclists/WordPress", Group: true},
	{Category: "SecLists / Vtiger", Name: "Vtiger shell plugin", Description: "Vtiger plugin shell collection", Path: "seclists/Vtiger", Group: true},
	{Category: "SecLists", Name: "backdoor_list", Description: "Backdoor reference list", Path: "seclists/backdoor_list.txt"},
	{Category: "SecLists / Laudanum", Name: "Laudanum 1.0", Description: "Multi-language Laudanum shell collection", Path: "seclists/laudanum-1.0", Group: true},
}

func Run(args []string, stdout, stderr io.Writer) error {
	return run(args, os.Stdin, stdout, stderr)
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 1 || args[1] == "ls" {
		return list(stdout, stderr)
	}
	if args[1] == "__complete" {
		return complete(stdout, stderr)
	}
	if args[1] == "show" {
		if len(args) != 3 {
			return errors.New("usage: xwebshell show <ID>")
		}
		return show(args[2], stdout, stderr)
	}
	if args[1] == "export" {
		if len(args) != 3 {
			return errors.New("usage: xwebshell export <ID>")
		}
		return exportEntry(args[2], stdin, stdout, stderr)
	}
	if args[1] == "-h" || args[1] == "--help" {
		return printHelp(stdout)
	}
	if args[1] == "-V" || args[1] == "--version" {
		_, err := fmt.Fprintf(stdout, "xwebshell %s\n", Version)
		return err
	}
	return errors.New("usage: xwebshell [ls|show <ID>|export <ID>]")
}

func list(stdout, stderr io.Writer) error {
	if _, err := os.Stat(webshellRoot); err != nil {
		return fmt.Errorf("webshells package not found: %w", err)
	}

	entries, err := listEntries(stderr)
	if err != nil {
		return err
	}

	available, missing, newCount := 0, 0, 0
	for _, entry := range entries {
		switch entry.status {
		case "+":
			available++
		case "!":
			missing++
		case "?":
			newCount++
		}
	}
	_, _ = fmt.Fprintln(stdout, "Web shell catalog")
	_, _ = fmt.Fprintf(stdout, "Available: %d  New: %d  Missing: %d\n\n", available, newCount, missing)
	_, _ = fmt.Fprintln(stdout, "ID  St  Category                Name")
	_, _ = fmt.Fprintln(stdout, "--  --  ---------------------  ------------------------------")
	for _, entry := range entries {
		_, _ = fmt.Fprintf(stdout, "%2d  [%s] %-21s  %s\n", entry.id, entry.status, entry.Category, entry.Name)
	}
	return nil
}

func printHelp(stdout io.Writer) error {
	_, err := fmt.Fprintln(stdout, `usage: xwebshell [ls|show <ID>|export <ID>]

List and inspect Kali web shell templates.

commands:
  ls                 list templates and their status
  show <ID>          show paths and contents for a template
  export <ID>        configure and copy a template to the current directory

options:
  -h, --help         show this help
  -V, --version      show version`)
	return err
}

func complete(stdout, stderr io.Writer) error {
	entries, err := listEntries(stderr)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err := fmt.Fprintf(stdout, "%d\t%s / %s\t%s\n", entry.id, entry.Category, entry.Name, entry.Description); err != nil {
			return err
		}
	}
	return nil
}

func exportEntry(value string, stdin io.Reader, stdout, stderr io.Writer) error {
	if _, err := os.Stat(webshellRoot); err != nil {
		return fmt.Errorf("webshells package not found: %w", err)
	}
	entries, err := listEntries(stderr)
	if err != nil {
		return err
	}
	id, err := strconv.Atoi(value)
	if err != nil || id < 1 || id > len(entries) {
		return fmt.Errorf("unknown web shell ID: %s", value)
	}
	entry := entries[id-1]
	if entry.status != "+" {
		return fmt.Errorf("web shell ID %d is not available: [%s]", id, entry.status)
	}

	sources, err := filesForEntry(entry)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return fmt.Errorf("web shell ID %d has no files", id)
	}

	root := filepath.Join(".", filepath.Base(entry.Path))
	destinations := make([]string, 0, len(sources))
	if len(sources) == 1 && !entry.Group {
		destinations = append(destinations, root)
	} else {
		sourceRoot := filepath.Join(webshellRoot, entry.Path)
		for _, source := range sources {
			relative, relErr := filepath.Rel(sourceRoot, source)
			if relErr != nil {
				return relErr
			}
			destinations = append(destinations, filepath.Join(root, relative))
		}
	}

	for _, destination := range destinations {
		if _, statErr := os.Lstat(destination); statErr == nil {
			return fmt.Errorf("output already exists: %s", destination)
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
	}
	reader := bufio.NewReader(stdin)
	for i, source := range sources {
		if mkdirErr := os.MkdirAll(filepath.Dir(destinations[i]), 0755); mkdirErr != nil {
			return mkdirErr
		}
		content, readErr := os.ReadFile(source)
		if readErr != nil {
			return readErr
		}
		templatePath := filepath.ToSlash(entry.Path)
		if entry.Group {
			relative, relErr := filepath.Rel(filepath.Join(webshellRoot, entry.Path), source)
			if relErr != nil {
				return relErr
			}
			templatePath = filepath.ToSlash(filepath.Join(entry.Path, relative))
		}
		content, configureErr := configureTemplate(templatePath, content, reader, stdout)
		if configureErr != nil {
			return configureErr
		}
		if writeErr := writeFile(destinations[i], content, source); writeErr != nil {
			return writeErr
		}
	}
	_, err = fmt.Fprintf(stdout, "exported %s to %s\n", entry.Name, root)
	return err
}

func writeFile(destination string, content []byte, source string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if err := os.WriteFile(destination, content, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chmod(destination, info.Mode().Perm())
}

func configureTemplate(path string, content []byte, reader *bufio.Reader, stdout io.Writer) ([]byte, error) {
	text := string(content)
	switch path {
	case "perl/perl-reverse-shell.pl":
		ip, err := callbackIP(reader, stdout, findValue(text, `my \$ip = '([^']*)';`))
		if err != nil {
			return nil, err
		}
		port, err := prompt(reader, stdout, "Callback port", findValue(text, `my \$port = ([0-9]+);`))
		if err != nil {
			return nil, err
		}
		text = replaceValue(text, `my \$ip = '[^']*';`, "my $ip = '"+ip+"';")
		text = replaceValue(text, `my \$port = [0-9]+;`, "my $port = "+port+";")
	case "php/php-reverse-shell.php", "seclists/laudanum-1.0/wordpress/templates/php-reverse-shell.php":
		ip, err := callbackIP(reader, stdout, findValue(text, `\$ip\s*=\s*'([^']*)'`))
		if err != nil {
			return nil, err
		}
		port, err := prompt(reader, stdout, "Callback port", findValue(text, `\$port\s*=\s*([0-9]+)`))
		if err != nil {
			return nil, err
		}
		text = replaceValue(text, `\$ip\s*=\s*'[^']*'`, "$ip = '"+ip+"'")
		text = replaceValue(text, `\$port\s*=\s*[0-9]+`, "$port = "+port)
	case "seclists/Magento/newadmin-Inchoo.php":
		var err error
		text, err = configureMagentoInchoo(text, reader, stdout)
		if err != nil {
			return nil, err
		}
	case "seclists/Magento/newadmin-KINKCreative.php":
		var err error
		text, err = configureMagentoKink(text, reader, stdout)
		if err != nil {
			return nil, err
		}
	}
	return []byte(text), nil
}

func configureMagentoInchoo(text string, reader *bufio.Reader, stdout io.Writer) (string, error) {
	values := []struct {
		label, key, current string
	}{
		{"Username", "USERNAME", findValue(text, `USERNAME','([^']*)`)},
		{"Email", "EMAIL", findValue(text, `EMAIL','([^']*)`)},
		{"Password", "PASSWORD", findValue(text, `PASSWORD','([^']*)`)},
	}
	for _, item := range values {
		value, err := prompt(reader, stdout, item.label, item.current)
		if err != nil {
			return "", err
		}
		text = replaceValue(text, `(?m)^#define\('`+item.key+`','[^']*'\);`, "define('"+item.key+"','"+value+"');")
	}
	return text, nil
}

func configureMagentoKink(text string, reader *bufio.Reader, stdout io.Writer) (string, error) {
	values := []struct {
		label, key, current string
	}{
		{"Username", "username", findValue(text, `'username'\s*=>\s*'([^']*)'`)},
		{"First name", "firstname", findValue(text, `'firstname'\s*=>\s*'([^']*)'`)},
		{"Last name", "lastname", findValue(text, `'lastname'\s*=>\s*'([^']*)'`)},
		{"Email", "email", findValue(text, `'email'\s*=>\s*'([^']*)'`)},
		{"Password", "password", findValue(text, `'password'\s*=>\s*'([^']*)'`)},
	}
	for _, item := range values {
		value, err := prompt(reader, stdout, item.label, item.current)
		if err != nil {
			return "", err
		}
		text = replaceCaptureValue(text, `('`+item.key+`'\s*=>\s*')[^']*(')`, value)
	}
	return text, nil
}

func prompt(reader *bufio.Reader, stdout io.Writer, label, defaultValue string) (string, error) {
	if _, err := fmt.Fprintf(stdout, "%s [%s]: ", label, defaultValue); err != nil {
		return "", err
	}
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		value = defaultValue
	}
	if strings.ContainsAny(value, "\r\n'") {
		return "", fmt.Errorf("invalid value for %s", label)
	}
	return value, nil
}

func callbackIP(reader *bufio.Reader, stdout io.Writer, fallback string) (string, error) {
	if ip := detectCallbackIP(); ip != "" {
		_, err := fmt.Fprintf(stdout, "Callback IP: %s\n", ip)
		return ip, err
	}
	return prompt(reader, stdout, "Callback IP", fallback)
}

func detectCallbackIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := networkInterface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.To4().String()
			}
		}
	}
	return ""
}

func findValue(text, expression string) string {
	match := regexp.MustCompile(expression).FindStringSubmatch(text)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func replaceValue(text, expression, replacement string) string {
	return regexp.MustCompile(expression).ReplaceAllStringFunc(text, func(string) string {
		return replacement
	})
}

func replaceCaptureValue(text, expression, value string) string {
	return regexp.MustCompile(expression).ReplaceAllStringFunc(text, func(match string) string {
		parts := regexp.MustCompile(expression).FindStringSubmatch(match)
		return parts[1] + value + parts[2]
	})
}

func show(value string, stdout, stderr io.Writer) error {
	if _, err := os.Stat(webshellRoot); err != nil {
		return fmt.Errorf("webshells package not found: %w", err)
	}
	entries, err := listEntries(stderr)
	if err != nil {
		return err
	}
	id, err := strconv.Atoi(value)
	if err != nil || id < 1 || id > len(entries) {
		return fmt.Errorf("unknown web shell ID: %s", value)
	}
	entry := entries[id-1]
	_, _ = fmt.Fprintf(stdout, "ID: %d\nCategory: %s\nName: %s\nDescription: %s\nStatus: [%s]\n\n", entry.id, entry.Category, entry.Name, entry.Description, entry.status)

	paths, err := filesForEntry(entry)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		_, err = fmt.Fprintf(stdout, "Path: %s\n", filepath.Join(webshellRoot, entry.Path))
		return err
	}
	for i, path := range paths {
		if i > 0 {
			_, _ = fmt.Fprintln(stdout)
		}
		_, _ = fmt.Fprintf(stdout, "Path: %s\n\n", path)
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			_, _ = fmt.Fprintf(stdout, "Unable to read: %s\n", readErr)
			continue
		}
		_, _ = stdout.Write(content)
		if len(content) == 0 || content[len(content)-1] != '\n' {
			_, _ = fmt.Fprintln(stdout)
		}
	}
	return nil
}

func listEntries(stderr io.Writer) ([]listedEntry, error) {
	entries := make([]listedEntry, 0, len(catalog))
	known := make(map[string]catalogEntry, len(catalog))
	for _, item := range catalog {
		known[item.Path] = item
		status := "+"
		if _, err := os.Stat(filepath.Join(webshellRoot, item.Path)); err != nil {
			status = "!"
		}
		entries = append(entries, listedEntry{catalogEntry: item, status: status})
	}
	newPaths, err := discoverNewPaths(known)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: failed to inspect web shell files: %s\n", err)
		return nil, err
	}
	for _, path := range newPaths {
		entries = append(entries, listedEntry{catalogEntry: catalogEntry{Category: newCategory(path), Name: filepath.Base(path), Description: "Unregistered web shell file", Path: path}, status: "?"})
	}
	for i := range entries {
		entries[i].id = i + 1
	}
	return entries, nil
}

func filesForEntry(entry listedEntry) ([]string, error) {
	root := filepath.Join(webshellRoot, entry.Path)
	info, err := os.Stat(root)
	if err != nil {
		return nil, nil
	}
	if !info.IsDir() {
		return []string{root}, nil
	}
	paths := make([]string, 0)
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info != nil && !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	return paths, err
}

func newCategory(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) > 1 {
		return "New / " + parts[0]
	}
	return "New / unsupported"
}

func discoverNewPaths(known map[string]catalogEntry) ([]string, error) {
	paths := make([]string, 0)
	roots := []struct {
		path   string
		prefix string
	}{
		{path: webshellRoot, prefix: ""},
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Join(webshellRoot, "seclists")); err == nil {
		roots = append(roots, struct {
			path   string
			prefix string
		}{path: resolved, prefix: "seclists"})
	}
	seen := make(map[string]struct{})
	for _, root := range roots {
		err := filepath.Walk(root.path, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info == nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 || path == root.path {
				return nil
			}
			relative, err := filepath.Rel(root.path, path)
			if err != nil {
				return err
			}
			if root.prefix != "" {
				relative = filepath.Join(root.prefix, relative)
			}
			if coveredByCatalog(relative, known) {
				return nil
			}
			seen[relative] = struct{}{}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func coveredByCatalog(path string, known map[string]catalogEntry) bool {
	if _, ok := known[path]; ok {
		return true
	}
	for knownPath, item := range known {
		if item.Group && strings.HasPrefix(path, knownPath+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}
