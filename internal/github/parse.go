package github

import (
	"fmt"
	"regexp"
	"strings"
)

// httpsRepo matches https://github.com/owner/repo(.git)? and
// https://<host>/owner/repo(.git)?
var httpsRepo = regexp.MustCompile(`^https?://[^/]+/([^/]+)/([^/]+?)(?:\.git)?/?$`)

// sshRepo matches git@github.com:owner/repo(.git)?
var sshRepo = regexp.MustCompile(`^[^@]+@[^:]+:([^/]+)/([^/]+?)(?:\.git)?/?$`)

// ParseRepo extracts the owner and repository name from a git clone URL.
// Recognizes both HTTPS (https://github.com/owner/repo.git) and SSH
// (git@github.com:owner/repo.git) forms and tolerates a trailing slash or
// missing .git suffix.
func ParseRepo(url string) (owner, repo string, err error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", "", fmt.Errorf("empty repository URL")
	}
	for _, re := range []*regexp.Regexp{httpsRepo, sshRepo} {
		if m := re.FindStringSubmatch(url); m != nil {
			return m[1], m[2], nil
		}
	}
	return "", "", fmt.Errorf("unrecognized git URL format: %q", url)
}
