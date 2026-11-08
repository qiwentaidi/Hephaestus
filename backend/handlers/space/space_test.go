package space

import (
	"strings"
	"testing"

	"hephaestus-backend/pkg/types"
)

func TestValidateSpaceQueryConstraints(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		opt     *types.SpaceOption
		wantErr string
	}{
		{
			name:  "nil option is allowed",
			query: strings.Repeat("a OR b ", 10),
			opt:   nil,
		},
		{
			name:  "hunter disabled skips the limit",
			query: strings.Repeat("a OR b ", 10),
			opt:   &types.SpaceOption{EnableHunter: false},
		},
		{
			name:  "hunter accepts five OR operators",
			query: "a OR b OR c OR d OR e OR f",
			opt:   &types.SpaceOption{EnableHunter: true},
		},
		{
			name:    "hunter rejects six OR operators",
			query:   "a OR b OR c OR d OR e OR f OR g",
			opt:     &types.SpaceOption{EnableHunter: true},
			wantErr: "最多只支持 5 个 OR",
		},
		{
			name:    "lowercase or is counted as well",
			query:   "a or b or c or d or e or f or g",
			opt:     &types.SpaceOption{EnableHunter: true},
			wantErr: "当前为 6 个",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpaceQueryConstraints(tt.query, tt.opt)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("期望通过校验，实际报错: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("期望报错包含 %q，实际通过校验", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("期望报错包含 %q，实际: %v", tt.wantErr, err)
			}
		})
	}
}
