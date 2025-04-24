package distribution

import "testing"

func TestImage(t *testing.T) {
	tests := []struct {
		distribution string
		version      string
		want         string
	}{
		{
			distribution: "redpanda-connect",
			version:      "4.103.1",
			want:         "docker.redpanda.com/redpandadata/connect:4.103.1",
		},
		{
			distribution: "redpanda-connect-cloud",
			version:      "4.103.1",
			want:         "docker.redpanda.com/redpandadata/connect:4.103.1-cloud",
		},
		{
			distribution: "benthos",
			version:      "4.27.0",
			want:         "ghcr.io/benthosdev/benthos:4.27.0",
		},
	}

	for _, test := range tests {
		t.Run(test.distribution, func(t *testing.T) {
			dist, ok := Lookup(test.distribution)
			if !ok {
				t.Fatalf("Lookup(%q) found nothing", test.distribution)
			}

			if got := dist.Image(test.version); got != test.want {
				t.Errorf("Image(%q) = %q, want %q", test.version, got, test.want)
			}
		})
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, ok := Lookup("nope"); ok {
		t.Error(`Lookup("nope") found a distribution`)
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name         string
		ref          string
		wantImage    string
		wantDistName string // empty means that Resolve returns no distribution
	}{
		{
			name:         "distribution and version",
			ref:          "redpanda-connect:4.103.1",
			wantImage:    "docker.redpanda.com/redpandadata/connect:4.103.1",
			wantDistName: "redpanda-connect",
		},
		{
			name:         "cloud distribution takes the suffix",
			ref:          "redpanda-connect-cloud:4.103.1",
			wantImage:    "docker.redpanda.com/redpandadata/connect:4.103.1-cloud",
			wantDistName: "redpanda-connect-cloud",
		},
		{
			// The library of an unknown image keeps the naming of the caller,
			// because the build behind the image is not known.
			name:      "reference of an image stays as it is",
			ref:       "ghcr.io/example/custom:1.0.0",
			wantImage: "ghcr.io/example/custom:1.0.0",
		},
		{
			name:      "reference with a port stays as it is",
			ref:       "localhost:5000/custom:1.0.0",
			wantImage: "localhost:5000/custom:1.0.0",
		},
		{
			name:      "reference with no tag stays as it is",
			ref:       "ghcr.io/example/custom",
			wantImage: "ghcr.io/example/custom",
		},
		{
			name:      "name of a distribution with no version stays as it is",
			ref:       "redpanda-connect",
			wantImage: "redpanda-connect",
		},
		{
			name:      "unknown name with a version stays as it is",
			ref:       "nope:1.0.0",
			wantImage: "nope:1.0.0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			image, dist := Resolve(test.ref)

			if image != test.wantImage {
				t.Errorf("image = %q, want %q", image, test.wantImage)
			}

			switch {
			case test.wantDistName == "":
				if dist != nil {
					t.Errorf("distribution = %q, want none", dist.Name)
				}
			case dist == nil:
				t.Errorf("distribution = none, want %q", test.wantDistName)
			case dist.Name != test.wantDistName:
				t.Errorf("distribution = %q, want %q", dist.Name, test.wantDistName)
			}
		})
	}
}
