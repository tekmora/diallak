package serverdef

import (
	"fmt"
	"testing"
)

func boolPtr(b bool) *bool {
	return &b
}

func addRole(def *Definition, r Role) {
	if r.Purpose == "" {
		r.Purpose = "Default purpose for test."
	}
	def.Roles = append(def.Roles, r)
}

func TestValidate_C1_ManagedAssignedBy(t *testing.T) {
	t.Run("positive platform and integration", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-managed-plat", Name: "Managed Plat", Kind: "managed",
			AssignedBy: "platform", Basis: "decided", Permissions: []string{},
		})
		addRole(def, Role{
			ID: "test-managed-integ", Name: "Managed Integ", Kind: "managed",
			AssignedBy: "integration", Basis: "decided", Permissions: []string{},
		})
		findings := Validate(def)
		if hasFinding(findings, CodeRoleGrant, "roles[test-managed-plat].assigned_by", "platform or integration") {
			t.Errorf("unexpected finding for platform assigned_by")
		}
		if hasFinding(findings, CodeRoleGrant, "roles[test-managed-integ].assigned_by", "platform or integration") {
			t.Errorf("unexpected finding for integration assigned_by")
		}
	})

	t.Run("negative operator assigned_by", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-managed", Name: "Managed Role", Kind: "managed",
			AssignedBy: "operator", Basis: "decided", Permissions: []string{},
		})
		findings := Validate(def)
		const want = "role kind managed must have assigned_by platform or integration"
		if !hasFinding(findings, CodeRoleGrant, "roles[test-managed].assigned_by", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})
}

func TestValidate_C2_IntegrationAssignedByOnlyForManaged(t *testing.T) {
	t.Run("negative integration assigned_by on recognition", func(t *testing.T) {
		def := valid(t)
		r := findRole(def.Roles, "contributor")
		if r == nil {
			t.Fatal("role contributor not found")
		}
		r.AssignedBy = "integration"
		findings := Validate(def)
		const want = "assigned_by integration is only for role kind managed"
		if !hasFinding(findings, CodeRoleGrant, "roles[contributor].assigned_by", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})

	t.Run("positive managed role with integration", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-managed", Name: "Managed Role", Kind: "managed",
			AssignedBy: "integration", Basis: "decided", Permissions: []string{},
		})
		findings := Validate(def)
		if hasFinding(findings, CodeRoleGrant, "roles[test-managed].assigned_by", "assigned_by integration is only for role kind managed") {
			t.Errorf("unexpected finding for managed role with integration")
		}
	})
}

func TestValidate_C3_EmptyPermissionsList(t *testing.T) {
	t.Run("negative managed with permission", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-managed", Name: "Managed Role", Kind: "managed",
			AssignedBy: "platform", Basis: "decided", Permissions: []string{"VIEW_CHANNEL"},
		})
		findings := Validate(def)
		const want = "role kind managed must have empty permissions list"
		if !hasFinding(findings, CodeNothingAttaches, "roles[test-managed].permissions", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})

	t.Run("negative notification with permission", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-notif", Name: "Notify Role", Kind: "notification",
			AssignedBy: "choice", Basis: "decided", Permissions: []string{"VIEW_CHANNEL"},
			Mentionable: boolPtr(true),
		})
		findings := Validate(def)
		const want = "role kind notification must have empty permissions list"
		if !hasFinding(findings, CodeNothingAttaches, "roles[test-notif].permissions", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})
}

func TestValidate_C4_ManagedNotInRoleOrder(t *testing.T) {
	t.Run("negative managed in role_order", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-managed", Name: "Managed Role", Kind: "managed",
			AssignedBy: "platform", Basis: "decided", Permissions: []string{},
		})
		def.RoleOrder = append(def.RoleOrder, "test-managed")
		findings := Validate(def)
		want := fmt.Sprintf("role %q of kind managed must not be listed in role_order", "test-managed")
		if !hasFinding(findings, CodeRoleOrder, "role_order[test-managed]", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})

	t.Run("positive managed not in role_order produces no finding", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-managed", Name: "Managed Role", Kind: "managed",
			AssignedBy: "platform", Basis: "decided", Permissions: []string{},
		})
		findings := Validate(def)
		if hasFinding(findings, CodeRoleOrder, "roles[test-managed]", "not listed in role_order") {
			t.Errorf("unexpected role_order missing finding for managed role")
		}
	})
}

func TestValidate_C5_ManagedNotNamedInForbiddenPlaces(t *testing.T) {
	setupManaged := func() (*Definition, string) {
		def := valid(t)
		const id = "test-managed"
		addRole(def, Role{
			ID: id, Name: "Managed Role", Kind: "managed",
			AssignedBy: "platform", Basis: "decided", Permissions: []string{},
		})
		return def, id
	}

	t.Run("channel access list", func(t *testing.T) {
		def, id := setupManaged()
		def.Channels[0].Access = append(def.Channels[0].Access, Access{
			Role:  id,
			Allow: []string{},
			Deny:  []string{},
		})
		findings := Validate(def)
		path := fmt.Sprintf("channels[%s].access[%s]", def.Channels[0].ID, id)
		if !hasFinding(findings, CodeRef, path, "is managed") {
			t.Errorf("missing access finding for managed role at %s; got %+v", path, findings)
		}
	})

	t.Run("channel owner", func(t *testing.T) {
		def, id := setupManaged()
		def.Channels[0].Owner = id
		findings := Validate(def)
		path := fmt.Sprintf("channels[%s].owner", def.Channels[0].ID)
		if !hasFinding(findings, CodeChannelOwner, path, "is managed") {
			t.Errorf("missing owner finding for managed role at %s; got %+v", path, findings)
		}
	})

	t.Run("area role", func(t *testing.T) {
		def, id := setupManaged()
		def.Areas[0].Role = id
		findings := Validate(def)
		path := fmt.Sprintf("areas[%s].role", def.Areas[0].ID)
		if !hasFinding(findings, CodeRef, path, "is managed") {
			t.Errorf("missing area role finding for managed role at %s; got %+v", path, findings)
		}
	})

	t.Run("choice option", func(t *testing.T) {
		def, id := setupManaged()
		def.Choices.Questions[0].Options = append(def.Choices.Questions[0].Options, Option{
			Title: "Test Managed Option",
			Role:  id,
		})
		findings := Validate(def)
		path := fmt.Sprintf("choices.questions[%s].options[%s]", def.Choices.Questions[0].ID, id)
		if !hasFinding(findings, CodeRoleGrant, path, "is managed") {
			t.Errorf("missing choice option finding for managed role at %s; got %+v", path, findings)
		}
	})
}

func TestValidate_C6_ManagedMentionable(t *testing.T) {
	t.Run("negative managed states mentionable", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-managed", Name: "Managed Role", Kind: "managed",
			AssignedBy: "platform", Basis: "decided", Permissions: []string{},
			Mentionable: boolPtr(false),
		})
		findings := Validate(def)
		const want = "role kind managed must not state mentionable"
		if !hasFinding(findings, CodeRoleGrant, "roles[test-managed].mentionable", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})

	t.Run("positive boundary managed states shown_apart true", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-managed", Name: "Managed Role", Kind: "managed",
			AssignedBy: "platform", Basis: "decided", Permissions: []string{},
			ShownApart: boolPtr(true),
		})
		findings := Validate(def)
		if hasFinding(findings, CodeRoleGrant, "roles[test-managed].shown_apart", "") {
			t.Errorf("unexpected finding for shown_apart on managed role")
		}
	})
}

func TestValidate_C7_EveryoneAndBotStateNeither(t *testing.T) {
	t.Run("everyone states shown_apart", func(t *testing.T) {
		def := valid(t)
		r := findRole(def.Roles, "everyone")
		r.ShownApart = boolPtr(true)
		findings := Validate(def)
		const want = "role kind everyone must not state shown_apart or mentionable"
		if !hasFinding(findings, CodeRoleGrant, "roles[everyone].shown_apart", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})

	t.Run("everyone states mentionable", func(t *testing.T) {
		def := valid(t)
		r := findRole(def.Roles, "everyone")
		r.Mentionable = boolPtr(false)
		findings := Validate(def)
		const want = "role kind everyone must not state shown_apart or mentionable"
		if !hasFinding(findings, CodeRoleGrant, "roles[everyone].mentionable", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})

	t.Run("bot states shown_apart", func(t *testing.T) {
		def := valid(t)
		r := findRole(def.Roles, "bot")
		r.ShownApart = boolPtr(true)
		findings := Validate(def)
		const want = "role kind bot must not state shown_apart or mentionable"
		if !hasFinding(findings, CodeRoleGrant, "roles[bot].shown_apart", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})

	t.Run("bot states mentionable", func(t *testing.T) {
		def := valid(t)
		r := findRole(def.Roles, "bot")
		r.Mentionable = boolPtr(true)
		findings := Validate(def)
		const want = "role kind bot must not state shown_apart or mentionable"
		if !hasFinding(findings, CodeRoleGrant, "roles[bot].mentionable", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})
}

func TestValidate_C8_NotificationConditions(t *testing.T) {
	t.Run("negative assigned_by not choice", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-notif", Name: "Notify Role", Kind: "notification",
			AssignedBy: "operator", Basis: "decided", Permissions: []string{},
			Mentionable: boolPtr(true),
		})
		findings := Validate(def)
		const want = "role kind notification must have assigned_by choice"
		if !hasFinding(findings, CodeRoleGrant, "roles[test-notif].assigned_by", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})

	t.Run("negative mentionable missing", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-notif", Name: "Notify Role", Kind: "notification",
			AssignedBy: "choice", Basis: "decided", Permissions: []string{},
		})
		findings := Validate(def)
		const want = "role kind notification must state mentionable true"
		if !hasFinding(findings, CodeRoleGrant, "roles[test-notif].mentionable", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})

	t.Run("negative boundary mentionable false", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-notif", Name: "Notify Role", Kind: "notification",
			AssignedBy: "choice", Basis: "decided", Permissions: []string{},
			Mentionable: boolPtr(false),
		})
		findings := Validate(def)
		const want = "role kind notification must state mentionable true"
		if !hasFinding(findings, CodeRoleGrant, "roles[test-notif].mentionable", want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})

	t.Run("negative channel access allow non-empty", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-notif", Name: "Notify Role", Kind: "notification",
			AssignedBy: "choice", Basis: "decided", Permissions: []string{},
			Mentionable: boolPtr(true),
		})
		def.Channels[0].Access = append(def.Channels[0].Access, Access{
			Role:  "test-notif",
			Allow: []string{"VIEW_CHANNEL"},
			Deny:  []string{},
		})
		findings := Validate(def)
		path := fmt.Sprintf("channels[%s].access[test-notif]", def.Channels[0].ID)
		const want = "notification role must have empty allow list"
		if !hasFinding(findings, CodeNothingAttaches, path, want) {
			t.Errorf("missing finding %q; got %+v", want, findings)
		}
	})

	t.Run("negative notification in 0 choices", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-notif", Name: "Notify Role", Kind: "notification",
			AssignedBy: "choice", Basis: "decided", Permissions: []string{},
			Mentionable: boolPtr(true),
		})
		def.RoleOrder = append(def.RoleOrder, "test-notif")
		findings := Validate(def)
		if !hasFinding(findings, CodeRoleGrant, "roles[test-notif]", "is the option of 0 choices; it must be the option of exactly one") {
			t.Errorf("missing choice coverage finding; got %+v", findings)
		}
	})

	t.Run("positive choice option grants notification", func(t *testing.T) {
		def := valid(t)
		addRole(def, Role{
			ID: "test-notif", Name: "Notify Role", Kind: "notification",
			AssignedBy: "choice", Basis: "decided", Permissions: []string{},
			Mentionable: boolPtr(true),
		})
		def.RoleOrder = append(def.RoleOrder, "test-notif")
		def.Choices.Questions[0].Options = append(def.Choices.Questions[0].Options, Option{
			Title: "Test Notification",
			Role:  "test-notif",
		})
		findings := Validate(def)
		path := fmt.Sprintf("choices.questions[%s].options[test-notif]", def.Choices.Questions[0].ID)
		if hasFinding(findings, CodeRoleGrant, path, "choice option grants role") {
			t.Errorf("unexpected choice option finding for notification role; got %+v", findings)
		}
	})

	t.Run("positive boundary shown_apart false on recognition role is valid", func(t *testing.T) {
		def := valid(t)
		r := findRole(def.Roles, "contributor")
		r.ShownApart = boolPtr(false)
		findings := Validate(def)
		if hasFinding(findings, CodeRoleGrant, "roles[contributor].shown_apart", "") {
			t.Errorf("unexpected finding for shown_apart false on recognition role; got %+v", findings)
		}
	})
}

func TestValidate_C9_NotificationBelowBotRole(t *testing.T) {
	def := valid(t)
	addRole(def, Role{
		ID: "test-notif", Name: "Notify Role", Kind: "notification",
		AssignedBy: "choice", Basis: "decided", Permissions: []string{},
		Mentionable: boolPtr(true),
	})
	// Insert test-notif before bot role in RoleOrder
	def.RoleOrder = append([]string{"test-notif"}, def.RoleOrder...)
	findings := Validate(def)
	want := fmt.Sprintf("role %q assigned by choice must stand below the bot role", "test-notif")
	if !hasFinding(findings, CodeRoleOrder, "role_order[test-notif]", want) {
		t.Errorf("missing finding %q; got %+v", want, findings)
	}
}
