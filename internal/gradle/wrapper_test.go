package gradle

import "testing"

func TestExtractWrapperVersion(t *testing.T) {
	cases := []struct {
		name    string
		content string
		version string
		ok      bool
	}{
		{"escaped protocol", `distributionUrl=https\://services.gradle.org/distributions/gradle-8.10.2-bin.zip` + "\n", "8.10.2", true},
		{"all distribution", `distributionUrl=https\://services.gradle.org/distributions/gradle-8.9-all.zip` + "\n", "8.9", true},
		{"release candidate", "distributionUrl=https://services.gradle.org/distributions/gradle-9.0.0-rc-1-bin.zip\n", "9.0.0-rc-1", true},
		{"snapshot", "distributionUrl=https://services.gradle.org/distributions/gradle-9.9.0-20260919063608+0000-bin.zip\n", "9.9.0-20260919063608+0000", true},
		{"comments and other keys", "# header\n! bang\norg.gradle.jvmargs=-Xmx1g\ndistributionUrl=https://example.com/gradle-8.10.1-bin.zip\n", "8.10.1", true},
		{"crlf", "distributionUrl=https://example.com/gradle-8.10.2-bin.zip\r\n", "8.10.2", true},
		{"no distributionUrl", "org.gradle.jvmargs=-Xmx1g\n", "", false},
		{"unparseable url", "distributionUrl=https://example.com/gradle-latest.zip\n", "", false},
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
