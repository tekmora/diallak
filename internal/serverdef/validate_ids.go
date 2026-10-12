package serverdef

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var idRegexp = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)

func checkItemIDs(v *validator, kind string, ids []string) {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		path := fmt.Sprintf("%s[%s]", kind, id)
		if !idRegexp.MatchString(id) {
			v.add(CodeID, path, fmt.Sprintf("%s id %q does not match regex ^[a-z][a-z0-9-]{1,31}$", kind, id))
		}
		if _, ok := seen[id]; ok {
			v.add(CodeID, path, fmt.Sprintf("duplicate %s id %q", kind, id))
		}
		seen[id] = struct{}{}
	}
}

func (v *validator) checkIDs() {
	var metricIDs, roleIDs, categoryIDs, channelIDs, areaIDs, questionIDs, autoIDs []string
	for _, m := range v.def.Metrics {
		metricIDs = append(metricIDs, m.ID)
	}
	for _, r := range v.def.Roles {
		roleIDs = append(roleIDs, r.ID)
	}
	for _, c := range v.def.Categories {
		categoryIDs = append(categoryIDs, c.ID)
	}
	for _, ch := range v.def.Channels {
		channelIDs = append(channelIDs, ch.ID)
	}
	for _, a := range v.def.Areas {
		areaIDs = append(areaIDs, a.ID)
	}
	for _, q := range v.def.Choices.Questions {
		questionIDs = append(questionIDs, q.ID)
	}
	for _, auto := range v.def.Automation {
		autoIDs = append(autoIDs, auto.ID)
	}

	checkItemIDs(v, "metrics", metricIDs)
	checkItemIDs(v, "roles", roleIDs)
	checkItemIDs(v, "categories", categoryIDs)
	checkItemIDs(v, "channels", channelIDs)
	checkItemIDs(v, "areas", areaIDs)
	checkItemIDs(v, "choices.questions", questionIDs)
	checkItemIDs(v, "automation", autoIDs)
}

func (v *validator) checkTexts() {
	if strings.TrimSpace(v.def.Summary) == "" {
		v.add(CodeText, "summary", "summary must not be empty after trimming")
	}
	v.checkMetricAndRoleTexts()
	v.checkCategoryTexts()
	v.checkChannelTexts()
	v.checkAreaTexts()
	v.checkChoiceTexts()
	v.checkAutomationTexts()
}

func (v *validator) checkMetricAndRoleTexts() {
	for _, m := range v.def.Metrics {
		p := fmt.Sprintf("metrics[%s]", m.ID)
		if strings.TrimSpace(m.Measures) == "" {
			v.add(CodeText, p+".measures", "measures must not be empty")
		}
		if strings.TrimSpace(m.Source) == "" {
			v.add(CodeText, p+".source", "source must not be empty")
		}
	}
	for _, r := range v.def.Roles {
		p := fmt.Sprintf("roles[%s]", r.ID)
		if strings.TrimSpace(r.Name) == "" {
			v.add(CodeText, p+".name", "role name must not be empty")
		}
		if strings.TrimSpace(r.Purpose) == "" {
			v.add(CodeText, p+".purpose", "role purpose must not be empty")
		}
	}
}

func (v *validator) checkCategoryTexts() {
	for _, c := range v.def.Categories {
		p := fmt.Sprintf("categories[%s]", c.ID)
		if strings.TrimSpace(c.Name) == "" {
			v.add(CodeText, p+".name", "category name must not be empty")
		}
		if strings.TrimSpace(c.Purpose) == "" {
			v.add(CodeText, p+".purpose", "category purpose must not be empty")
		}
	}
}

func (v *validator) checkChannelTexts() {
	for _, ch := range v.def.Channels {
		p := fmt.Sprintf("channels[%s]", ch.ID)
		nameRunes := utf8.RuneCountInString(ch.Name)
		if strings.TrimSpace(ch.Name) == "" || nameRunes < 1 || nameRunes > MaxChannelNameRunes {
			v.add(CodeText, p+".name", "channel name must have between 1 and 100 characters")
		}
		if strings.TrimSpace(ch.Purpose) == "" {
			v.add(CodeText, p+".purpose", "channel purpose must not be empty")
		}
		checkTagTexts(v, p, ch.Tags)
	}
}

func checkTagTexts(v *validator, channelPath string, tags []Tag) {
	for _, tag := range tags {
		tagRunes := utf8.RuneCountInString(tag.Name)
		if strings.TrimSpace(tag.Name) == "" || tagRunes < 1 || tagRunes > MaxTagNameRunes {
			v.add(CodeText, fmt.Sprintf("%s.tags[%s]", channelPath, tag.Name),
				"tag name must have between 1 and 20 characters")
		}
	}
}

func (v *validator) checkAreaTexts() {
	for _, a := range v.def.Areas {
		p := fmt.Sprintf("areas[%s]", a.ID)
		if strings.TrimSpace(a.Name) == "" {
			v.add(CodeText, p+".name", "area name must not be empty")
		}
		if a.Opening != nil && strings.TrimSpace(a.Opening.Reasoning) == "" {
			v.add(CodeText, p+".opening.reasoning", "opening reasoning must not be empty")
		}
		if a.Closing != nil && strings.TrimSpace(a.Closing.Reasoning) == "" {
			v.add(CodeText, p+".closing.reasoning", "closing reasoning must not be empty")
		}
	}
}

func (v *validator) checkAutomationTexts() {
	for _, auto := range v.def.Automation {
		if strings.TrimSpace(auto.What) == "" {
			v.add(CodeText, fmt.Sprintf("automation[%s].what", auto.ID), "what must not be empty")
		}
	}
}

func inSet(val string, allowed []string) bool {
	for _, a := range allowed {
		if val == a {
			return true
		}
	}
	return false
}

func (v *validator) checkEnums() {
	roleKinds := []string{"everyone", "operator", "moderator", "bot", "recognition", "interest", "language", "notification", "managed"}
	roleAssignedBy := []string{"platform", "operator", "choice", "bot", "integration"}
	roleBases := []string{"decided", "proposal"}
	for _, r := range v.def.Roles {
		p := fmt.Sprintf("roles[%s]", r.ID)
		if !inSet(r.Kind, roleKinds) {
			v.add(CodeEnum, p+".kind", fmt.Sprintf("invalid role kind %q", r.Kind))
		}
		if !inSet(r.AssignedBy, roleAssignedBy) {
			v.add(CodeEnum, p+".assigned_by", fmt.Sprintf("invalid assigned_by %q", r.AssignedBy))
		}
		if !inSet(r.Basis, roleBases) {
			v.add(CodeEnum, p+".basis", fmt.Sprintf("invalid basis %q", r.Basis))
		}
	}
	v.checkChannelAndAreaEnums()
	v.checkAutomationEnums()
	v.checkChoiceEnums()
}

func (v *validator) checkChannelAndAreaEnums() {
	chTypes := []string{"text", "forum", "announcement", "voice"}
	chOpens := []string{"start", "threshold"}
	for _, ch := range v.def.Channels {
		p := fmt.Sprintf("channels[%s]", ch.ID)
		if !inSet(ch.Type, chTypes) {
			v.add(CodeEnum, p+".type", fmt.Sprintf("invalid channel type %q", ch.Type))
		}
		if !inSet(ch.Opens, chOpens) {
			v.add(CodeEnum, p+".opens", fmt.Sprintf("invalid channel opens %q", ch.Opens))
		}
	}
	areaKinds := []string{"topic", "language"}
	areaOnClose := []string{"hide"}
	for _, a := range v.def.Areas {
		p := fmt.Sprintf("areas[%s]", a.ID)
		if !inSet(a.Kind, areaKinds) {
			v.add(CodeEnum, p+".kind", fmt.Sprintf("invalid area kind %q", a.Kind))
		}
		if !inSet(a.OnClose, areaOnClose) {
			v.add(CodeEnum, p+".on_close", fmt.Sprintf("invalid area on_close %q", a.OnClose))
		}
		v.checkRuleEnums(p+".opening", a.Opening)
		v.checkRuleEnums(p+".closing", a.Closing)
	}
}

func (v *validator) checkRuleEnums(path string, rule *Rule) {
	if rule == nil {
		return
	}
	if rule.Comparison != "at-least" {
		v.add(CodeEnum, path+".comparison", fmt.Sprintf("invalid comparison %q", rule.Comparison))
	}
	if rule.Basis != "proposal" && rule.Basis != "measured" {
		v.add(CodeEnum, path+".basis", fmt.Sprintf("invalid rule basis %q", rule.Basis))
	}
}

func (v *validator) checkAutomationEnums() {
	autoBy := []string{"bot", "platform", "operator", "moderator"}
	autoState := []string{"built", "decided", "planned", "observed"}
	for _, auto := range v.def.Automation {
		p := fmt.Sprintf("automation[%s]", auto.ID)
		if !inSet(auto.By, autoBy) {
			v.add(CodeEnum, p+".by", fmt.Sprintf("invalid automation by %q", auto.By))
		}
		if !inSet(auto.State, autoState) {
			v.add(CodeEnum, p+".state", fmt.Sprintf("invalid automation state %q", auto.State))
		}
	}
}

func (v *validator) checkVersion() {
	if v.def.Version != 1 {
		v.add(CodeVersion, "version", fmt.Sprintf("version must be 1, found %d", v.def.Version))
	}
	if v.def.Applied {
		v.add(CodeVersion, "applied", "applied must be false")
	}
}

func (v *validator) checkSingletons() {
	counts := make(map[string]int)
	for _, r := range v.def.Roles {
		counts[r.Kind]++
	}
	for _, single := range []string{"everyone", "operator", "bot"} {
		if counts[single] != 1 {
			v.add(CodeSingleton, "roles", fmt.Sprintf("expected exactly one %s role, found %d",
				single, counts[single]))
		}
	}
}

func (v *validator) checkReferences() {
	catMap := make(map[string]struct{})
	for _, c := range v.def.Categories {
		catMap[c.ID] = struct{}{}
	}
	roleMap := make(map[string]Role, len(v.def.Roles))
	for _, r := range v.def.Roles {
		roleMap[r.ID] = r
	}
	areaMap := make(map[string]struct{})
	for _, a := range v.def.Areas {
		areaMap[a.ID] = struct{}{}
	}
	metricMap := make(map[string]struct{})
	for _, m := range v.def.Metrics {
		metricMap[m.ID] = struct{}{}
	}

	v.checkChannelRefs(catMap, areaMap, roleMap)
	v.checkAreaRefs(roleMap, metricMap)
	v.checkChoiceRefs(roleMap)
}

func (v *validator) checkChannelRefs(catMap, areaMap map[string]struct{}, roleMap map[string]Role) {
	for _, ch := range v.def.Channels {
		p := fmt.Sprintf("channels[%s]", ch.ID)
		if _, ok := catMap[ch.Category]; !ok {
			v.add(CodeRef, p+".category", fmt.Sprintf("unknown category %q", ch.Category))
		}
		if ch.Area != "" {
			if _, ok := areaMap[ch.Area]; !ok {
				v.add(CodeRef, p+".area", fmt.Sprintf("unknown area %q", ch.Area))
			}
		}
		v.checkChannelAccessRefs(p, ch.Access, roleMap)
	}
}

func (v *validator) checkChannelAccessRefs(channelPath string, access []Access, roleMap map[string]Role) {
	for _, acc := range access {
		p := fmt.Sprintf("%s.access[%s]", channelPath, acc.Role)
		r, ok := roleMap[acc.Role]
		if !ok {
			v.add(CodeRef, p, fmt.Sprintf("unknown access role %q", acc.Role))
			continue
		}
		if r.Kind == "managed" {
			v.add(CodeRef, p, fmt.Sprintf("role %q is managed", acc.Role))
		}
	}
}

func (v *validator) checkAreaRefs(roleMap map[string]Role, metricMap map[string]struct{}) {
	for _, a := range v.def.Areas {
		p := fmt.Sprintf("areas[%s]", a.ID)
		r, ok := roleMap[a.Role]
		if !ok {
			v.add(CodeRef, p+".role", fmt.Sprintf("unknown area role %q", a.Role))
		} else if r.Kind == "managed" {
			v.add(CodeRef, p+".role", fmt.Sprintf("role %q is managed", a.Role))
		}
		if a.Opening != nil {
			if _, ok := metricMap[a.Opening.Metric]; !ok {
				v.add(CodeRef, p+".opening.metric", fmt.Sprintf("unknown metric %q", a.Opening.Metric))
			}
		}
		if a.Closing != nil {
			if _, ok := metricMap[a.Closing.Metric]; !ok {
				v.add(CodeRef, p+".closing.metric", fmt.Sprintf("unknown metric %q", a.Closing.Metric))
			}
		}
	}
}

func (v *validator) checkChannelOwner() {
	roleMap := make(map[string]Role)
	for _, r := range v.def.Roles {
		roleMap[r.ID] = r
	}
	for _, ch := range v.def.Channels {
		p := fmt.Sprintf("channels[%s].owner", ch.ID)
		if ch.Owner == "" {
			v.add(CodeChannelOwner, p, "channel owner must not be empty")
			continue
		}
		role, ok := roleMap[ch.Owner]
		if !ok {
			v.add(CodeChannelOwner, p, fmt.Sprintf("channel owner %q not found", ch.Owner))
			continue
		}
		if role.Kind == "managed" {
			v.add(CodeChannelOwner, p, fmt.Sprintf("role %q is managed", role.ID))
			continue
		}
		if role.Kind != "operator" && role.Kind != "moderator" {
			v.add(CodeChannelOwner, p, fmt.Sprintf("channel owner must be operator or moderator, found %s", role.Kind))
		}
	}
}
