package service

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// SecurityRefusal is the fixed, safe response for a confidently identified
// extraction or permission-bypass attempt. It never mentions detection rules.
const SecurityRefusal = "I can't help with that request. I'm the SwiftDrop Delivery Assistant, and I can help with SwiftDrop services, delivery pricing, tracking and your own deliveries."

// Security categories are coarse and safe to log. They never contain the
// message body.
const (
	SecurityPromptExtraction   = "prompt_extraction"
	SecuritySecretExtraction   = "secret_extraction"
	SecurityCrossUserAccess    = "cross_user_access"
	SecurityRoleImpersonation  = "role_impersonation"
	SecurityPermissionBypass   = "permission_bypass"
	SecurityBulkDataExtraction = "bulk_data_extraction"
	SecurityInternalData       = "internal_data_extraction"
)

type securityRule struct {
	category   string
	expression *regexp.Regexp
	// operational permits the rule for verified DISPATCHER and ADMIN callers,
	// who are authorized to ask about parcels across customers.
	operational bool
}

var securityRules = []securityRule{
	{
		category: SecurityPromptExtraction,
		expression: regexp.MustCompile(
			`\b(ignore|disregard|forget|override|bypass)\b.{0,40}\b(previous|prior|above|earlier|all)\b.{0,30}\b(instructions?|rules?|prompts?|directions?)\b`),
	},
	{
		category: SecurityPromptExtraction,
		expression: regexp.MustCompile(
			`\b(system|hidden|developer|internal)\b.{0,20}\b(prompt|instructions?)\b`),
	},
	{
		category: SecurityPromptExtraction,
		expression: regexp.MustCompile(
			`\b(reveal|show|print|display|repeat|expose|leak|tell me)\b.{0,30}\b(prompt|system instructions?|hidden instructions?|internal instructions?)\b`),
	},
	{
		category: SecuritySecretExtraction,
		expression: regexp.MustCompile(
			`\b(reveal|show|print|display|expose|leak|give me|tell me|dump)\b.{0,40}\b(environment variables?|env vars?|api keys?|credentials?|access tokens?|secrets?)\b`),
	},
	{
		category: SecuritySecretExtraction,
		expression: regexp.MustCompile(
			`\b(what|which|where)\b.{0,25}\b(api keys?|access tokens?|credentials?|secrets?|gemini key)\b`),
	},
	{
		category:    SecurityCrossUserAccess,
		operational: true,
		expression: regexp.MustCompile(
			`\b(another|other|different|someone else'?s?|some other)\b.{0,30}\b(customer|user|client|account|person)\b.{0,30}\b(parcel|delivery|order|data|information|history|account)\b`),
	},
	{
		category:    SecurityCrossUserAccess,
		operational: true,
		expression: regexp.MustCompile(
			`\b(use|with|by)\b.{0,20}\b(another|other)\b.{0,20}\b(user|customer)\b.{0,15}\b(id|identity|account)\b`),
	},
	{
		category: SecurityRoleImpersonation,
		expression: regexp.MustCompile(
			`\b(pretend|act|behave|pose|roleplay)\b.{0,30}\b(i am|i'm|to be|as)\b.{0,20}\b(admin|administrator|dispatcher|staff|owner)\b`),
	},
	{
		category: SecurityPermissionBypass,
		expression: regexp.MustCompile(
			`\b(ignore|bypass|disable|turn off|override|skip)\b.{0,30}\b(authori[sz]ation|permissions?|security|access controls?|role checks?|ownership)\b`),
	},
	{
		category: SecurityPermissionBypass,
		expression: regexp.MustCompile(
			`\b(ignore|override)\b.{0,20}\b(your|the)\b.{0,15}\b(rules|restrictions|scope|security)\b`),
	},
	{
		category: SecurityPermissionBypass,
		expression: regexp.MustCompile(
			`\b(pretend|act as|behave as)\b.{0,20}\b(another|different)\b.{0,15}\b(role|user|customer)\b`),
	},
	{
		category: SecurityBulkDataExtraction,
		expression: regexp.MustCompile(
			`\b(list|show|return|give|dump|export|reveal)\b.{0,25}\b(all|every)\b.{0,20}\b(users|customers|accounts)\b`),
	},
	{
		category: SecurityBulkDataExtraction,
		expression: regexp.MustCompile(
			`\b(all|every)\b.{0,10}\b(users|customers)\b.{0,15}\b(database|records|data)\b`),
	},
	{
		category: SecurityInternalData,
		expression: regexp.MustCompile(
			`\b(show|dump|export|reveal|print)\b.{0,30}\b(database|schema|table|sql)\b`),
	},
}

// DetectSecurityThreat reports a coarse category for a confident, explicit
// extraction or permission-bypass attempt. It is a supplementary defence and
// never grants access. Cross-customer wording is permitted for verified
// DISPATCHER and ADMIN callers because their authorized tools may access
// operational parcels; everyone else is refused before the provider is called.
func DetectSecurityThreat(message, role string, authenticated bool) (string, bool) {
	normalized := normalizeSecurityText(message)
	if normalized == "" {
		return "", false
	}
	privileged := authenticated &&
		(role == string(auth.RoleDispatcher) || role == string(auth.RoleAdmin))

	for _, rule := range securityRules {
		if rule.operational && privileged {
			continue
		}
		if rule.expression.MatchString(normalized) {
			return rule.category, true
		}
	}
	return "", false
}

// normalizeSecurityText lowercases, strips invisible characters and control
// characters, normalises common punctuation and collapses whitespace so simple
// obfuscation cannot hide an explicit request.
func normalizeSecurityText(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\ufeff' || r == '\u00ad':
			continue
		case unicode.IsControl(r):
			builder.WriteByte(' ')
		case r == '\u2018' || r == '\u2019':
			builder.WriteByte('\'')
		case r == '\u201c' || r == '\u201d':
			builder.WriteByte('"')
		case r == '\u2013' || r == '\u2014' || r == '-' || r == '_':
			builder.WriteByte(' ')
		default:
			builder.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}
