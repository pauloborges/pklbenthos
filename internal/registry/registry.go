// Package registry lists the tags of a container image.
//
// It speaks the part of the OCI distribution API that a tag listing needs. A
// registry answers an anonymous request with a challenge, and the client then
// asks the named service for a token and tries again. Docker Hub and the
// GitHub container registry both work this way, so one client reaches both.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Reference is the host and the path of an image, without a tag.
type Reference struct {
	Host string
	Path string
}

// ParseReference splits the reference of an image, such as
// "docker.redpanda.com/redpandadata/connect".
//
// A reference with no host belongs to Docker Hub, and a reference of a single
// name belongs to the "library" namespace of Docker Hub, as with "alpine".
func ParseReference(image string) (Reference, error) {
	image = strings.TrimSuffix(image, "/")

	if image == "" {
		return Reference{}, fmt.Errorf("empty image reference")
	}

	host, path, found := strings.Cut(image, "/")

	// A first element that carries no dot and no port is a namespace of Docker
	// Hub, and not a host.
	if !found || (!strings.Contains(host, ".") && !strings.Contains(host, ":")) {
		return Reference{Host: "registry-1.docker.io", Path: "library/" + image}, nil
	}

	// docker.redpanda.com holds a mirror of Docker Hub, and answers with a
	// challenge that names the token service of Docker Hub. Ask Docker Hub
	// itself, so that one code path serves both.
	if host == "docker.redpanda.com" {
		host = "registry-1.docker.io"
	}

	if !strings.Contains(path, "/") && host == "registry-1.docker.io" {
		path = "library/" + path
	}

	return Reference{Host: host, Path: path}, nil
}

// semverTag matches a tag of three numbers, and takes the text after them as a
// suffix, as with "4.104.0-cloud".
var semverTag = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(.*)$`)

// Tags returns every tag of an image.
func Tags(ctx context.Context, client *http.Client, ref Reference) ([]string, error) {
	if client == nil {
		client = http.DefaultClient
	}

	next := fmt.Sprintf("https://%s/v2/%s/tags/list?n=1000", ref.Host, url.PathEscape(ref.Path))

	// A path holds slashes, which PathEscape turns into %2F. Put them back,
	// because the registry reads the path of a repository as it is.
	next = strings.ReplaceAll(next, "%2F", "/")

	var (
		token string
		tags  []string
	)

	for next != "" {
		body, link, err := get(ctx, client, next, &token)
		if err != nil {
			return nil, err
		}

		var page struct {
			Tags []string `json:"tags"`
		}

		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decode tags of %s: %w", ref.Path, err)
		}

		tags = append(tags, page.Tags...)

		next = ""

		if link != "" {
			target, err := url.Parse(link)
			if err == nil {
				next = (&url.URL{Scheme: "https", Host: ref.Host}).ResolveReference(target).String()
			}
		}
	}

	return tags, nil
}

// get reads a URL, and answers a challenge with a token. It holds the token in
// the caller, because every page of a listing needs it.
func get(ctx context.Context, client *http.Client, target string, token *string) (body []byte, nextLink string, err error) {
	send := func() (*http.Response, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}

		if *token != "" {
			request.Header.Set("Authorization", "Bearer "+*token)
		}

		return client.Do(request)
	}

	response, err := send()
	if err != nil {
		return nil, "", fmt.Errorf("list tags: %w", err)
	}

	if response.StatusCode == http.StatusUnauthorized {
		challenge := response.Header.Get("Www-Authenticate")
		response.Body.Close()

		got, err := fetchToken(ctx, client, challenge)
		if err != nil {
			return nil, "", err
		}

		*token = got

		if response, err = send(); err != nil {
			return nil, "", fmt.Errorf("list tags: %w", err)
		}
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("list tags: the registry answered %s", response.Status)
	}

	body, err = readAll(response)
	if err != nil {
		return nil, "", err
	}

	return body, parseNextLink(response.Header.Get("Link")), nil
}

// fetchToken reads a Bearer challenge and asks the named service for a token.
func fetchToken(ctx context.Context, client *http.Client, challenge string) (string, error) {
	if !strings.HasPrefix(challenge, "Bearer ") {
		return "", fmt.Errorf("the registry asks for %q, which this client does not speak", challenge)
	}

	fields := map[string]string{}

	for _, part := range splitChallenge(strings.TrimPrefix(challenge, "Bearer ")) {
		key, value, found := strings.Cut(part, "=")
		if !found {
			continue
		}

		fields[strings.TrimSpace(key)] = strings.Trim(value, `"`)
	}

	realm := fields["realm"]
	if realm == "" {
		return "", fmt.Errorf("the challenge of the registry names no realm")
	}

	query := url.Values{}
	for _, key := range []string{"service", "scope"} {
		if value := fields[key]; value != "" {
			query.Set(key, value)
		}
	}

	target := realm
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("get a token from %s: %w", realm, err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get a token from %s: the service answered %s", realm, response.Status)
	}

	body, err := readAll(response)
	if err != nil {
		return "", err
	}

	// A service answers with "token", and some answer with "access_token".
	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decode the token of %s: %w", realm, err)
	}

	if payload.Token != "" {
		return payload.Token, nil
	}

	if payload.AccessToken != "" {
		return payload.AccessToken, nil
	}

	return "", fmt.Errorf("%s gave no token", realm)
}

// splitChallenge cuts a challenge at every comma that sits outside a quoted
// string, because a scope holds commas of its own.
func splitChallenge(challenge string) []string {
	var (
		parts  []string
		start  int
		quoted bool
	)

	for i, char := range challenge {
		switch char {
		case '"':
			quoted = !quoted
		case ',':
			if !quoted {
				parts = append(parts, challenge[start:i])
				start = i + 1
			}
		}
	}

	return append(parts, challenge[start:])
}

// parseNextLink reads the Link header of a page and returns the target of the
// next one, or an empty string at the last page.
func parseNextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)

		target, rest, found := strings.Cut(part, ">")
		if !found || !strings.HasPrefix(target, "<") {
			continue
		}

		if strings.Contains(rest, `rel="next"`) || strings.Contains(rest, "rel=next") {
			return strings.TrimPrefix(target, "<")
		}
	}

	return ""
}
