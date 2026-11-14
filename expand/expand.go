// Package expand resolves $ references inside a string: $VAR, ${VAR},
// the six ${VAR<op>arg} operators, nested references, $$, $(command) and
// ${scheme:ref}.
//
// It knows nothing about any file format. A format hands it one string plus
// callbacks: Lookup says what a name holds, Run executes a command, Resolve
// answers a scheme reference. Rules that belong to a format stay in the format:
// which quoting suppresses expansion (dotenv: single quotes), which definition a
// self-reference sees (dotenv: only the ones above it), which error type a
// ${VAR:?msg} produces.
//
// Hand-written rather than os.Expand, which cannot express two things Docker
// Compose requires: `$$` as a literal dollar, and nested references such as
// ${VAR:-${FALLBACK:-x}} (os.Expand stops at the FIRST closing brace).
package expand

import (
	"errors"
	"fmt"
	"strings"
)

// DefaultMaxDepth bounds how deep a chain of references may nest.
//
// Real chains are one hop deep, so 32 is generous. Depth alone does not
// guarantee termination: A=$A$A branches at every level, 2^32 expansions at
// depth 32. Cycles are therefore cut by name through the Stack, and the depth
// bound is only a backstop for deep acyclic chains.
const DefaultMaxDepth = 32

// AtLookup marks a Var that came from outside the document (a Lookuper), not
// from a position in it.
const AtLookup = -1

// Op is the operator after a variable name inside ${...}.
type Op int

// The operators. Colon forms treat an empty value like an unset one.
const (
	OpNone         Op = iota // ${VAR}
	OpDefault                // ${VAR-default}   default when unset
	OpDefaultEmpty           // ${VAR:-default}  default when unset or empty
	OpAlternate              // ${VAR+alt}       alt when set
	OpAlternateSet           // ${VAR:+alt}      alt when set and non-empty
	OpRequired               // ${VAR?err}       error when unset
	OpRequiredSet            // ${VAR:?err}      error when unset or empty
)

// Ref is a parsed ${...} reference.
type Ref struct {
	Name string
	Op   Op
	// Arg is the text after the operator: a default, an alternate or an error
	// message, depending on Op.
	Arg string
}

// opSeps lists every operator's spelling. Colon forms come first because all
// six share their trailing character: checking ":-" before "-" is what keeps
// ${VAR:-d} from parsing as a "-" default named "VAR:".
var opSeps = []struct {
	sep string
	op  Op
}{
	{":-", OpDefaultEmpty},
	{":+", OpAlternateSet},
	{":?", OpRequiredSet},
	{"-", OpDefault},
	{"+", OpAlternate},
	{"?", OpRequired},
}

// ParseRef splits the inside of ${...}, or the name after a bare $, into a
// name and its operator. Variable names contain none of ':', '-', '+', '?', so
// the first such run starts the operator.
func ParseRef(tok string) Ref {
	for _, o := range opSeps {
		if n, a, ok := strings.Cut(tok, o.sep); ok {
			return Ref{Name: n, Op: o.op, Arg: a}
		}
	}
	return Ref{Name: tok}
}

// Var is what Lookup reports for a name.
type Var struct {
	// Value is the raw value, expanded in turn unless Literal.
	Value string
	// Set is false when nothing defines the name.
	Set bool
	// Literal values are inserted as written, never expanded again (dotenv:
	// a single-quoted entry; a value a plugin already resolved).
	Literal bool
	// At is where the value came from (a position the format understands,
	// e.g. an entry index) or AtLookup. It is pushed on the Stack while the
	// value expands, so a reference back into the same name can see it.
	At int
}

// Stack is the set of names currently being expanded, each with the At of the
// value being expanded. A Lookup consults it to apply its format's
// self-reference rule; the engine uses it to cut cycles.
//
// The zero Stack is not usable; use NewStack.
type Stack struct {
	m map[string]int
}

// NewStack returns an empty stack.
func NewStack() *Stack { return &Stack{m: map[string]int{}} }

// At reports whether name is being expanded, and from where.
func (s *Stack) At(name string) (at int, ok bool) {
	at, ok = s.m[name]
	return at, ok
}

// Enter records that name is being expanded from at and returns the function
// that restores the previous state. A name already on the stack is re-pointed,
// not refused; the format's Lookup decides what a re-entry sees, and must only
// ever move At strictly so every chain ends (dotenv: strictly up the file).
func (s *Stack) Enter(name string, at int) (leave func()) {
	prev, had := s.m[name]
	s.m[name] = at
	return func() {
		if had {
			s.m[name] = prev
		} else {
			delete(s.m, name)
		}
	}
}

// RequiredError reports a ${VAR:?message} or ${VAR?message} whose variable was
// not satisfied. These forms exist to fail loudly; yielding "" would defeat
// the only reason an author writes one.
type RequiredError struct {
	Key     string
	Message string
	// Empty distinguishes ":?" (unset OR empty) from "?" (unset only).
	Empty bool
}

func (e *RequiredError) Error() string {
	reason := "is not set"
	if e.Empty {
		reason = "is not set or is empty"
	}
	if e.Message == "" {
		return fmt.Sprintf("expand: required variable %s %s", e.Key, reason)
	}
	return fmt.Sprintf("expand: required variable %s %s: %s", e.Key, reason, e.Message)
}

// CommandError reports a $(command) whose runner returned an error: the
// command failed, exited non-zero, or the runner refused it. Cmd is the command
// text after its own references were expanded, as handed to the runner.
type CommandError struct {
	Cmd string
	// Err is the runner's error.
	Err error
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("expand: command %q failed: %v", e.Cmd, e.Err)
}

// Unwrap returns the runner's error.
func (e *CommandError) Unwrap() error { return e.Err }

// ErrStrict is matched by every error strict mode adds (*UnsetError,
// *UnknownSchemeError, *CommandOffError), so a caller can tell "a reference
// did not resolve" from a plugin or command failure with one errors.Is.
// ds:def id=errstrict-wtxc9pe2 owner=@maintainers stability=stable
var ErrStrict = errors.New("expand: unresolved reference")

// UnsetError reports, in strict mode, a reference to a name no source defines:
// $VAR, ${VAR}, or a ${VAR...} form whose operator does not handle unset
// (${VAR:-d}, ${VAR-d}, ${VAR+a} and ${VAR:+a} do, and ${VAR:?m} keeps its own
// *RequiredError). It names the variable and where the reference sits, never a
// value.
// ds:def id=unseterror-fzc9dngm owner=@maintainers stability=stable
type UnsetError struct {
	// Name is the variable that has no value.
	Name string
	// Offset is the byte offset, in the string handed to Expand, of the $ that
	// starts the outermost reference being expanded when this one failed. For a
	// reference written directly in that string it is the reference itself;
	// for one reached through another variable's value or an operator argument
	// it is the reference that led there (see Via).
	Offset int
	// Via is the variable whose value held the failing reference, or "" when
	// the reference is in the input itself or in an operator argument.
	Via string
}

func (e *UnsetError) Error() string {
	return fmt.Sprintf("expand: variable %s is not set (offset %d%s)", e.Name, e.Offset, via(e.Via))
}

// Is matches ErrStrict.
func (e *UnsetError) Is(target error) bool { return target == ErrStrict }

// UnknownSchemeError reports, in strict mode, a ${scheme:ref} that no installed
// Resolver claims and that no source defines as a name either, for example
// ${vault:db} with no vault plugin, or the typo ${gopas:x}. Offset and Via
// are as for UnsetError.
// ds:def id=unknownschemeerror-fg4srxup owner=@maintainers stability=stable
type UnknownSchemeError struct {
	// Scheme is the scheme as written, lowercased (see SplitScheme).
	Scheme string
	// Ref is the text after the colon.
	Ref    string
	Offset int
	Via    string
}

func (e *UnknownSchemeError) Error() string {
	return fmt.Sprintf("expand: no resolver for scheme %q in ${%s:%s} (offset %d%s)", e.Scheme, e.Scheme, e.Ref, e.Offset, via(e.Via))
}

// Is matches ErrStrict.
func (e *UnknownSchemeError) Is(target error) bool { return target == ErrStrict }

// CommandOffError reports, in strict mode, a $(command) met while command
// substitution is off (Engine.Run is nil). Outside strict mode the text is
// kept as written; strict refuses it because the caller would otherwise
// receive the literal "$(gopass show x)" where it expected a secret. Cmd is
// the command text as written, before its own references expand. Offset and
// Via are as for UnsetError.
// ds:def id=commandofferror-j9abapdg owner=@maintainers stability=stable
type CommandOffError struct {
	Cmd    string
	Offset int
	Via    string
}

func (e *CommandOffError) Error() string {
	return fmt.Sprintf("expand: command substitution is off: $(%s) (offset %d%s)", e.Cmd, e.Offset, via(e.Via))
}

// Is matches ErrStrict.
func (e *CommandOffError) Is(target error) bool { return target == ErrStrict }

// via renders the Via suffix of a strict error message.
func via(name string) string {
	if name == "" {
		return ""
	}
	return ", in the value of " + name
}

// Engine expands strings. Lookup is required; every other field is optional.
type Engine struct {
	// Lookup reports what name holds. It is called with the current Stack so
	// it can apply the format's self-reference rule. An error aborts.
	Lookup func(name string, st *Stack) (Var, error)

	// Resolve answers ${scheme:ref}. ok=false means "not my scheme", and the
	// token is then read as an ordinary reference, so installing a resolver
	// never changes how other values read. An error aborts.
	Resolve func(scheme, ref string) (value string, ok bool, err error)

	// Run executes $(command). nil keeps the text verbatim, so a value
	// survives a round trip instead of being blanked. An error from Run (a
	// command that fails, exits non-zero or is refused) aborts with a
	// *CommandError naming the command: a failed lookup of a secret must not
	// read as an empty value.
	Run func(cmd string) (string, error)

	// Observe sees every completed resolution (name, what it became).
	Observe func(name, resolved string)

	// Required builds the error for an unsatisfied ${VAR?msg}; nil returns
	// *RequiredError.
	Required func(name, message string, empty bool) error

	// MaxDepth overrides DefaultMaxDepth when > 0.
	MaxDepth int

	// Strict turns every reference that would otherwise expand to "" or stay
	// verbatim into an error, for a caller that needs certainty (a token read
	// from a config file must not silently become ""):
	//
	//   - an unset name with no operator, or with an operator that does not
	//     handle unset: *UnsetError. ${VAR:-d} and ${VAR-d} still use their
	//     default, ${VAR+a} and ${VAR:+a} still yield "" when unset (that is
	//     what they ask for), ${VAR:?m} still returns its *RequiredError, and
	//     a name that is set to "" is set, not an error;
	//   - a ${scheme:ref} no Resolver claims and no source defines:
	//     *UnknownSchemeError;
	//   - a $(command) while Run is nil: *CommandOffError.
	//
	// Every strict error matches ErrStrict. Off by default: the zero Engine
	// keeps the Compose behaviour. Text that is not a reference at all stays
	// as written in both modes: $$, a lone or trailing $, and an unbalanced
	// ${ or $( (there is no reference to resolve, so nothing is uncertain).
	Strict bool
}

// Expand resolves every reference in s. st may be nil; a caller expanding the
// value of a named entry pushes that name first (st.Enter) so a reference
// back to it is recognised as a cycle.
func (e *Engine) Expand(s string, st *Stack) (string, error) {
	if st == nil {
		st = NewStack()
	}
	return e.expand(s, 0, st, where{})
}

// where is the position context strict errors report: the offset of the
// outermost reference in the string handed to Expand, and the variable whose
// value is being expanded ("" for the input itself). At depth 0 the offset is
// taken per reference; deeper calls inherit it.
type where struct {
	offset int
	via    string
}

func (e *Engine) maxDepth() int {
	if e.MaxDepth > 0 {
		return e.MaxDepth
	}
	return DefaultMaxDepth
}

func (e *Engine) expand(s string, depth int, st *Stack, w where) (string, error) {
	if depth >= e.maxDepth() || !strings.ContainsRune(s, '$') {
		return s, nil
	}

	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			b.WriteByte('$') // a trailing $ is just a dollar sign
			continue
		}

		if depth == 0 {
			w.offset = i
		}
		switch next := s[i+1]; {
		case next == '$':
			b.WriteByte('$') // $$ is the escape for a literal dollar
			i++

		case next == '(':
			consumed, out, err := e.command(s[i:], depth, st, w)
			if err != nil {
				return "", err
			}
			b.WriteString(out)
			i += consumed - 1

		case next == '{':
			end := MatchBrace(s[i+1:])
			if end < 0 {
				// Unbalanced: emit verbatim rather than guess.
				b.WriteByte('$')
				continue
			}
			out, err := e.token(s[i+2:i+1+end], depth, st, w)
			if err != nil {
				return "", err
			}
			b.WriteString(out)
			i += end + 1

		case IsNameStart(next):
			n := 1
			for i+1+n < len(s) && IsNameChar(s[i+1+n]) {
				n++
			}
			out, err := e.resolve(ParseRef(s[i+1:i+1+n]), depth, st, w, "")
			if err != nil {
				return "", err
			}
			b.WriteString(out)
			i += n

		default:
			b.WriteByte('$') // $ followed by anything else is a dollar
		}
	}
	return b.String(), nil
}

// command handles a $(command) starting at s[0] and returns how many bytes it
// consumed.
func (e *Engine) command(s string, depth int, st *Stack, w where) (consumed int, out string, err error) {
	end := MatchParen(s[1:])
	if end < 0 {
		return 1, "$", nil // unbalanced: a literal dollar
	}
	whole := s[:end+2]
	if e.Run == nil {
		if e.Strict {
			return 0, "", &CommandOffError{Cmd: s[2 : end+1], Offset: w.offset, Via: w.via}
		}
		return len(whole), whole, nil
	}
	// The command may itself reference variables.
	cmd, err := e.expand(s[2:end+1], depth+1, st, w)
	if err != nil {
		return 0, "", err
	}
	result, runErr := e.Run(cmd)
	if runErr != nil {
		return 0, "", &CommandError{Cmd: cmd, Err: runErr}
	}
	return len(whole), result, nil
}

// token evaluates the inside of ${...}: a scheme reference when a resolver
// claims it, an ordinary reference otherwise.
//
// Strict mode treats an unclaimed ${scheme:ref} as one opaque name: it is
// looked up whole (so installing or removing a resolver never changes how a
// defined name reads) and, when unset, reported as *UnknownSchemeError, since
// "no resolver for vault" is the useful diagnosis. It is never split on an
// operator: outside strict mode ${vault:db-pass} with no vault plugin reads as
// the name "vault:db" with the default "pass", which is exactly the silent
// guess strict mode exists to refuse.
func (e *Engine) token(tok string, depth int, st *Stack, w where) (string, error) {
	scheme, ref, isScheme := SplitScheme(tok)
	if isScheme && e.Resolve != nil {
		v, handled, err := e.Resolve(scheme, ref)
		if err != nil {
			return "", err
		}
		if handled {
			return e.observed(scheme+":"+ref, v, nil)
		}
	}
	if isScheme && e.Strict {
		return e.resolve(Ref{Name: tok}, depth, st, w, scheme)
	}
	return e.resolve(ParseRef(tok), depth, st, w, "")
}

// resolve evaluates one parsed reference. unknownScheme is the scheme of a
// ${scheme:ref} token no resolver claimed ("" otherwise); strict mode reports
// an unset name from such a token as *UnknownSchemeError.
func (e *Engine) resolve(ref Ref, depth int, st *Stack, w where, unknownScheme string) (string, error) {
	v, err := e.Lookup(ref.Name, st)
	if err != nil {
		return "", err
	}
	hasValue := v.Set && v.Value != ""

	// useArg picks the operator's argument over the variable's own value. The
	// argument belongs to the REFERENCING value, so it expands in that
	// context, not as the variable.
	useArg := false
	switch ref.Op {
	case OpDefault:
		useArg = !v.Set
	case OpDefaultEmpty:
		useArg = !hasValue
	case OpAlternate:
		if !v.Set {
			return e.observed(ref.Name, "", nil)
		}
		useArg = true
	case OpAlternateSet:
		if !hasValue {
			return e.observed(ref.Name, "", nil)
		}
		useArg = true
	case OpRequired:
		if !v.Set {
			return "", e.required(ref.Name, ref.Arg, false)
		}
	case OpRequiredSet:
		if !hasValue {
			return "", e.required(ref.Name, ref.Arg, true)
		}
	}

	// An unresolved reference with no operator expands to "", never an error:
	// Compose treats an unset variable as empty. Strict mode refuses it; the
	// operators above have already handled every form that asks for unset to
	// mean something (default, alternate, required).
	if !useArg && !v.Set && e.Strict {
		return "", e.unsetError(ref.Name, unknownScheme, w)
	}
	var out string
	switch {
	case useArg:
		out, err = e.expand(ref.Arg, depth+1, st, w)
	case !v.Set:
	case v.Literal:
		out = v.Value
	default:
		leave := st.Enter(ref.Name, v.At)
		out, err = e.expand(v.Value, depth+1, st, where{offset: w.offset, via: ref.Name})
		leave()
	}
	return e.observed(ref.Name, out, err)
}

// unsetError builds strict mode's error for a name that has no value: an
// *UnknownSchemeError when the token was a ${scheme:ref} nobody claimed, an
// *UnsetError otherwise.
func (e *Engine) unsetError(name, unknownScheme string, w where) error {
	if unknownScheme != "" {
		_, ref, _ := strings.Cut(name, ":")
		return &UnknownSchemeError{Scheme: unknownScheme, Ref: ref, Offset: w.offset, Via: w.via}
	}
	return &UnsetError{Name: name, Offset: w.offset, Via: w.via}
}

func (e *Engine) required(name, msg string, empty bool) error {
	if e.Required != nil {
		return e.Required(name, msg, empty)
	}
	return &RequiredError{Key: name, Message: msg, Empty: empty}
}

func (e *Engine) observed(name, resolved string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if e.Observe != nil {
		e.Observe(name, resolved)
	}
	return resolved, nil
}

// SplitScheme splits "scheme:ref" when tok starts with a scheme (a letter,
// then letters, digits, '+', '.', '-') followed by ':' and the next byte is not
// an operator character ('-', '+', '?'), so ${VAR:-d} is never a scheme.
func SplitScheme(tok string) (scheme, ref string, ok bool) {
	i := strings.IndexByte(tok, ':')
	if i <= 0 || !isSchemeStart(tok[0]) {
		return "", "", false
	}
	for j := 1; j < i; j++ {
		if !isSchemeChar(tok[j]) {
			return "", "", false
		}
	}
	if i+1 < len(tok) && strings.IndexByte("-+?", tok[i+1]) >= 0 {
		return "", "", false
	}
	return strings.ToLower(tok[:i]), tok[i+1:], true
}

// MatchBrace returns the index of the '}' closing the '{' at s[0], counting
// nesting, or -1 when unbalanced. ${A:-${B:-c}} needs the OUTER brace.
func MatchBrace(s string) int { return match(s, '{', '}') }

// MatchParen returns the index of the ')' closing the '(' at s[0], counting
// nesting, or -1 when unbalanced.
func MatchParen(s string) int { return match(s, '(', ')') }

func match(s string, open, close byte) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// IsNameStart reports whether c may begin a bare $NAME.
func IsNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// IsNameChar reports whether c may continue a bare $NAME.
func IsNameChar(c byte) bool {
	return IsNameStart(c) || (c >= '0' && c <= '9')
}

func isSchemeStart(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isSchemeChar(c byte) bool {
	return isSchemeStart(c) || (c >= '0' && c <= '9') || c == '+' || c == '.' || c == '-'
}
