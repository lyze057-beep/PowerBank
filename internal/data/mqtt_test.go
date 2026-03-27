package data

import "testing"

func TestNormalizeTopic(t *testing.T) {
	cases := []struct {
		prefix string
		topic  string
		want   string
	}{
		{prefix: "powerbank/user", topic: "heartbeat", want: "powerbank/user/heartbeat"},
		{prefix: "powerbank/user/", topic: "/heartbeat/", want: "powerbank/user/heartbeat"},
		{prefix: "", topic: "heartbeat", want: "heartbeat"},
		{prefix: "powerbank", topic: "", want: "powerbank"},
	}
	for _, tc := range cases {
		got := normalizeTopic(tc.prefix, tc.topic)
		if got != tc.want {
			t.Fatalf("normalizeTopic(%q,%q)=%q, want %q", tc.prefix, tc.topic, got, tc.want)
		}
	}
}
