package distribution

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pauloborges/pklbenthos/internal/registry"
)

// releaseTag matches a tag of three numbers and nothing else, such as
// "4.103.1". A tag with more after the numbers is a variant, such as
// "4.103.1-cloud" or "4.103.1-rc1", and a distribution selects its own variant
// through its tag suffix.
var releaseTag = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

// Versions returns every released version of a distribution, from the oldest
// to the newest.
//
// A version is a tag of the image of the distribution that carries three
// numbers and the tag suffix of the distribution, and nothing else. A release
// candidate, a moving tag such as "latest", and the variant of another
// distribution all stay out.
func Versions(ctx context.Context, client *http.Client, dist *Distribution) ([]string, error) {
	ref, err := registry.ParseReference(dist.imageName)
	if err != nil {
		return nil, fmt.Errorf("read the image of %s: %w", dist.Name, err)
	}

	tags, err := registry.Tags(ctx, client, ref)
	if err != nil {
		return nil, fmt.Errorf("list the versions of %s: %w", dist.Name, err)
	}

	var versions []string

	for _, tag := range tags {
		version, ok := strings.CutSuffix(tag, dist.tagSuffix)
		if !ok && dist.tagSuffix != "" {
			continue
		}

		// Without a suffix of its own, a distribution must not take the tags
		// of a variant that shares the repository.
		if dist.tagSuffix == "" && !releaseTag.MatchString(tag) {
			continue
		}

		if !releaseTag.MatchString(version) {
			continue
		}

		versions = append(versions, version)
	}

	slices.SortFunc(versions, CompareVersions)

	return slices.Compact(versions), nil
}

// CompareVersions orders two versions of three numbers, so that 4.9.0 comes
// before 4.10.0. A plain string order would put them the other way round.
func CompareVersions(a, b string) int {
	fieldsA, fieldsB := releaseTag.FindStringSubmatch(a), releaseTag.FindStringSubmatch(b)

	if fieldsA == nil || fieldsB == nil {
		return cmp.Compare(a, b)
	}

	for i := 1; i <= 3; i++ {
		numberA, _ := strconv.Atoi(fieldsA[i])
		numberB, _ := strconv.Atoi(fieldsB[i])

		if numberA != numberB {
			return cmp.Compare(numberA, numberB)
		}
	}

	return 0
}

// IsVersion tells if a string is a version of three numbers.
func IsVersion(s string) bool {
	return releaseTag.MatchString(s)
}
