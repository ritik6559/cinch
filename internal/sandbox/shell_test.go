package sandbox

import (
	"slices"
	"strings"
	"testing"
)

func TestSplitFindsEverySimpleCommand(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"plain", "go test ./...", []string{"go test ./..."}},
		{"and", "go build && go test", []string{"go build", "go test"}},
		{"or", "go test || echo failed", []string{"go test", "echo failed"}},
		{"semicolon", "go vet; go test", []string{"go vet", "go test"}},
		{"pipe", "cat go.mod | head -3", []string{"cat go.mod", "head -3"}},
		{"background", "sleep 1 & echo done", []string{"sleep 1", "echo done"}},
		{"newline", "go vet\ngo test", []string{"go vet", "go test"}},
		{"three deep", "a && b || c", []string{"a", "b", "c"}},
		{"trailing operator", "go test &&", []string{"go test"}},
		{"empty", "", nil},
		{"only spaces", "   ", nil},

		{"quoted and", `echo "a && b"`, []string{`echo "a && b"`}},
		{"quoted pipe", `grep 'a|b' file`, []string{`grep 'a|b' file`}},
		{"quoted semicolon", `echo "one; two"`, []string{`echo "one; two"`}},
		{"escaped semicolon", `find . -exec ls \; `, []string{`find . -exec ls \;`}},

		{"stderr to stdout", "go build 2>&1", []string{"go build 2>&1"}},
		{"append", "go test >> out.log", []string{"go test >> out.log"}},
		{"both streams", "go test &> out.log", []string{"go test &> out.log"}},
		{"redirect then pipe", "go build 2>&1 | tee log", []string{"go build 2>&1", "tee log"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := Split(tt.in)
			if !slices.Equal(got, tt.want) {
				t.Errorf("Split(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitRefusesWhatItCannotRead(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"substitution", "go build $(cat args)", "substitution"},
		{"backticks", "go build `cat args`", "substitution"},
		{"substitution in quotes", `echo "$(whoami)"`, "substitution"},
		{"backticks in quotes", "echo \"`whoami`\"", "substitution"},
		{"brace expansion", "echo ${PAYLOAD}", "substitution"},
		{"eval", "eval $PAYLOAD", "eval"},
		{"nested shell", "bash -c 'rm -rf /'", "bash"},
		{"sh", "sh script.sh", "sh"},
		{"source", ". ./env.sh", "runtime"},
		{"exec after pipe", "go test | exec something", "exec"},
		{"command from a variable", "$TOOL --version", "variable"},
		{"unbalanced double", `echo "oops`, "unbalanced"},
		{"unbalanced single", "echo 'oops", "unbalanced"},
		{"redirect home", "echo x > ~/.bashrc", "~/.bashrc"},
		{"redirect absolute", "echo x > /etc/passwd", "/etc/passwd"},
		{"redirect parent", "echo x > ../outside.txt", "../outside.txt"},
		{"append home", "echo x >> ~/.profile", "~/.profile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, reason := Split(tt.in)
			if reason == "" {
				t.Fatalf("Split(%q) reported no reason, want one mentioning %q", tt.in, tt.want)
			}
			if !strings.Contains(reason, tt.want) {
				t.Errorf("Split(%q) reason = %q, want it to mention %q", tt.in, reason, tt.want)
			}
		})
	}
}

func TestSplitAcceptsOrdinaryCommands(t *testing.T) {
	for _, command := range []string{
		"go test ./...",
		"go build 2>&1",
		"go test > out.log",
		"go test >> logs/out.log",
		"cat internal/agent/agent.go",
		"grep -rn maxSteps internal",
		"git status --short",
		"npm run build && npm test",
		`echo "a && b"`,
		"ls -la | head",
	} {
		if _, reason := Split(command); reason != "" {
			t.Errorf("Split(%q) refused: %s", command, reason)
		}
	}
}
