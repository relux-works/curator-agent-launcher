package memberwire

// DescriptorNumbers contains syntax-checked dynamic numbers only. It neither
// opens nor validates handles or their underlying identities and ownership.
type DescriptorNumbers struct{ Request, Authorization, Objects, State, Response, Controller int }

// ParseDescriptorArgs parses only the arguments after the reserved member entry.
// It does not read process arguments, descriptors, environment, or any live state.
func ParseDescriptorArgs(args []string) (DescriptorNumbers, *Error) {
	bad := func() (DescriptorNumbers, *Error) {
		return DescriptorNumbers{}, &Error{"bootstrap", "descriptor_invalid"}
	}
	if len(args) != 12 {
		return bad()
	}
	var values [6]int
	var seen [6]bool
	var used [256]bool
	for i := 0; i < len(args); i += 2 {
		var index int
		switch args[i] {
		case "--request-fd":
			index = 0
		case "--authorization-fd":
			index = 1
		case "--objects-fd":
			index = 2
		case "--state-fd":
			index = 3
		case "--response-fd":
			index = 4
		case "--controller-fd":
			index = 5
		default:
			return bad()
		}
		if seen[index] {
			return bad()
		}
		s := args[i+1]
		if len(s) == 0 || len(s) > 3 || s[0] == '0' {
			return bad()
		}
		n := 0
		for j := range len(s) {
			if s[j] < '0' || s[j] > '9' {
				return bad()
			}
			n = n*10 + int(s[j]-'0')
		}
		if n < 3 || n > 255 || used[n] {
			return bad()
		}
		used[n], seen[index], values[index] = true, true, n
	}
	return DescriptorNumbers{values[0], values[1], values[2], values[3], values[4], values[5]}, nil
}
