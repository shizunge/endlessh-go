package main

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
)

func main() {
	message, err := readMessage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading message: %v\n", err)
		return
	}

	ip := extractIP(message)
	comment := fmt.Sprintf("SSH login attempts (endlessh): %s", message)

	fmt.Printf("reporting IP:'%s' - %s\n", ip, comment)

	if isCached(ip) {
		fmt.Printf("%s is still cached!\n", ip)
		return
	}

	reportToAbuseIPDB(ip, comment)
	appendToReportedIPs(ip)
}

func readMessage() (string, error) {
	var message string
	_, err := fmt.Scanln(&message)
	return message, err
}

func extractIP(message string) string {
	re := regexp.MustCompile(`(?<=host=::ffff:).*(?= port)`)
	return re.FindString(message)
}

func isCached(ip string) bool {
	_, err := os.Exec("cache", []string{strconv.Itoa(900), "echo", ip})
	return err == nil
}

func reportToAbuseIPDB(ip, comment string) {
	apiKey := os.Getenv("ABUSE_IPDB_API_KEY")
	url := "https://api.abuseipdb.com/api/v2/report"
	body := fmt.Sprintf("ip=%s&categories=18,22&comment=%s", ip, comment)
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating request: %v\n", err)
		return
	}
	req.Header.Set("Key", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println(resp.Status)
}

func appendToReportedIPs(ip string) {
	f, err := os.OpenFile("reportedIps.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening file: %v\n", err)
		return
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, ip)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing to file: %v\n", err)
	}
}
