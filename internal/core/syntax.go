package core

import "github.com/longafterx/gocloc/internal/lexer"

func syntaxForLanguage(language *Language) *lexer.Rules {
	if len(language.regexLineComments) != 0 {
		return nil // Caller-supplied regexes must retain their legacy semantics.
	}
	if language.syntax != nil {
		return language.syntax
	}
	// Caller-created C and Go definitions historically used the corrected
	// scanner when their comment markers matched the built-in definitions.
	switch language.Name {
	case "Go", "C", "C Header":
	default:
		return nil
	}
	rules := lexer.BuiltIn(language.Name)
	if !rules.MatchesLegacy(
		language.lineComments,
		language.multiLines,
		len(language.regexLineComments) != 0,
	) {
		return nil
	}
	return rules
}
