package resolve

// globMatch reports whether s matches the OSCAL "matching" glob pattern.
// '*' matches any run of characters (including none) and '?' matches exactly
// one character; every other character matches itself. Matching is on runes
// and runs in O(len(pattern)*len(s)) worst case without recursion.
func globMatch(pattern, s string) bool {
	p := []rune(pattern)
	r := []rune(s)
	pi, si := 0, 0
	star, mark := -1, 0
	for si < len(r) {
		switch {
		case pi < len(p) && (p[pi] == '?' || p[pi] == r[si]):
			pi++
			si++
		case pi < len(p) && p[pi] == '*':
			star = pi
			mark = si
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			si = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}
