package devid

import (
	"context"
	"strings"
	"testing"
)

func TestCaptureUsesInjectedReporterAndDoesNotRunLegacyGenerator(t *testing.T) {
	opts := generatorFixture(t, `// DS2API_DEVID_CAPTURE_V1
if (!process.argv.includes('--capture')) throw Error('missing capture flag');
console.log(JSON.stringify({ok:true,report:{url:'https://fp-it-acc.portal101.cn/deviceprofile/v4',headers:{Origin:'https://chat.deepseek.com'},body:'{}'}}));`)
	calls := 0
	opts.Reporter = func(_ context.Context, report ReportRequest) (int, []byte, error) {
		calls++
		if report.Headers["Origin"] != "https://chat.deepseek.com" {
			t.Fatal("SDK headers lost")
		}
		return 200, []byte(`{"code":1100,"detail":{"deviceId":"cloud-issued"}}`), nil
	}
	id, err := Generate(context.Background(), opts)
	if err != nil || id != "Bcloud-issued" || calls != 1 {
		t.Fatalf("capture/report: %q %v calls=%d", id, err, calls)
	}
	old := generatorFixture(t, "throw Error('legacy generator must never execute')")
	old.Reporter = opts.Reporter
	if _, err := Generate(context.Background(), old); err == nil || !strings.Contains(err.Error(), "too old") {
		t.Fatalf("legacy runner executed: %v", err)
	}
}

func TestReportRejectsUnexpectedTargetsAndServiceErrors(t *testing.T) {
	for _, target := range []string{"http://fp-it-acc.portal101.cn/deviceprofile/v4", "https://localhost/deviceprofile/v4", "https://fp-it-acc.portal101.cn/deviceprofile/v4?redirect=x"} {
		if err := ValidateReport(ReportRequest{URL: target, Body: "{}"}); err == nil {
			t.Fatalf("accepted %s", target)
		}
	}
	for _, sample := range []struct {
		status int
		body   string
	}{{500, `{"code":1100,"detail":{"deviceId":"id"}}`}, {200, `{}`}, {200, `invalid`}} {
		if _, err := deviceIDFromReport(sample.status, []byte(sample.body)); err == nil {
			t.Fatalf("accepted failed service response: %+v", sample)
		}
	}
}
