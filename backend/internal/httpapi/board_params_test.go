package httpapi

import "testing"

func TestBoardParamsDefaultsAndRefuses(t *testing.T) {
	bad, other := Model("x"), Format(9)
	glicko, n4 := Glicko2Daneo, N4
	tests := []struct {
		name       string
		run        int64
		model      *Model
		format     *Format
		wantModel  Model
		wantFormat Format
		wantMsg    string
	}{
		{"neither given: the defaults", 1, nil, nil, Glicko2Daneo, N0, ""},
		{"both given", 1, &glicko, &n4, Glicko2Daneo, 4, ""},
		{"no run", 0, nil, nil, Glicko2Daneo, N0, "bad parameter: run"},
		{"an unknown model", 1, &bad, nil, bad, N0, "unknown model"},
		{"an unknown format", 1, nil, &other, Glicko2Daneo, other, "unknown format"},
		{"the run is refused first", 0, &bad, &other, bad, other, "bad parameter: run"},
		{"then the model", 1, &bad, &other, bad, other, "unknown model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, format, msg := boardParams(tt.run, tt.model, tt.format)
			if msg != tt.wantMsg {
				t.Errorf("boardParams message = %q, want %q", msg, tt.wantMsg)
			}
			if tt.wantMsg == "" && (model != tt.wantModel || format != tt.wantFormat) {
				t.Errorf("boardParams = %q, %d, want %q, %d", model, format, tt.wantModel, tt.wantFormat)
			}
		})
	}
}
