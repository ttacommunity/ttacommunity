package notify

import "testing"

func TestChannelPermissionsFollowDiscordsOrder(t *testing.T) {
	const guild, bot, botRole, other = "400000000000000001", "100000000000000009", "600000000000000001", "600000000000000002"
	roles := map[string]uint64{guild: viewChannel, botRole: createPrivateThreads | sendInThreads, other: manageThreads}
	tests := []struct {
		name       string
		owner      string
		roles      map[string]uint64
		memberOf   []string
		overwrites []overwrite
		want       uint64
	}{
		{"its roles and @everyone's", "", roles, []string{botRole}, nil, viewChannel | createPrivateThreads | sendInThreads},
		{"only roles it has", "", roles, nil, nil, viewChannel},
		{"the server's owner", bot, roles, nil, nil, allPermissions},
		{"an administrator", "", map[string]uint64{guild: 0, botRole: administrator}, []string{botRole}, []overwrite{{ID: guild, Deny: allPermissions}}, allPermissions},
		{"@everyone denied the channel", "", roles, []string{botRole}, []overwrite{{ID: guild, Deny: viewChannel}}, createPrivateThreads | sendInThreads},
		{"its role allowed what @everyone is denied", "", roles, []string{botRole},
			[]overwrite{{ID: guild, Deny: viewChannel}, {ID: botRole, Allow: viewChannel}}, viewChannel | createPrivateThreads | sendInThreads},
		{"one role denying what another allows: the allow wins", "", roles, []string{botRole, other},
			[]overwrite{{ID: botRole, Deny: manageThreads}, {ID: other, Allow: manageThreads}}, viewChannel | createPrivateThreads | sendInThreads | manageThreads},
		{"its own overwrite last", "", roles, []string{botRole},
			[]overwrite{{ID: botRole, Allow: viewChannel}, {ID: bot, Kind: 1, Deny: viewChannel}}, createPrivateThreads | sendInThreads},
		{"a role's overwrite for a role it lacks", "", roles, []string{botRole}, []overwrite{{ID: other, Deny: createPrivateThreads}}, viewChannel | createPrivateThreads | sendInThreads},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := channelPermissions(guild, tt.owner, bot, tt.roles, tt.memberOf, tt.overwrites)
			if got != tt.want {
				t.Errorf("channelPermissions = %b, want %b", got, tt.want)
			}
		})
	}
}
