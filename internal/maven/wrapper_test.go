package maven

import "testing"

func TestExtractWrapperVersion(t *testing.T) {
	cases := []struct {
		name    string
		content string
		version string
		ok      bool
	}{
		{"standard", "distributionUrl=https://repo.maven.apache.org/maven2/org/apache/maven/apache-maven/3.9.11/apache-maven-3.9.11-bin.zip\n", "3.9.11", true},
		{"two segment", "distributionUrl=https://example.com/apache-maven-3.9-bin.tar.gz", "3.9", true},
		{"comments and other keys", "# header\nwrapperVersion=3.3.2\ndistributionType=only-script\ndistributionUrl=https://example.com/apache-maven-4.0.0-bin.zip\n", "4.0.0", true},
		{"crlf", "distributionUrl=https://example.com/apache-maven-3.9.9-bin.zip\r\n", "3.9.9", true},
		{"no distributionUrl", "wrapperVersion=3.3.2\n", "", false},
		{"unparseable url", "distributionUrl=https://example.com/maven-latest.zip\n", "", false},
		{"empty", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, ok := ExtractWrapperVersion(c.content)
			if v != c.version || ok != c.ok {
				t.Errorf("got (%q, %v), want (%q, %v)", v, ok, c.version, c.ok)
			}
		})
	}
}
