package memberwire

import (
	"reflect"
	"testing"
)

func fdArgs() []string {
	return []string{"--request-fd", "3", "--authorization-fd", "4", "--objects-fd", "5", "--state-fd", "6", "--response-fd", "7", "--controller-fd", "255"}
}
func fdBad(t *testing.T, args []string) {
	t.Helper()
	v, e := ParseDescriptorArgs(args)
	if v != (DescriptorNumbers{}) || e == nil || e.Stage() != "bootstrap" || e.Code() != "descriptor_invalid" || e.Error() != "bootstrap/descriptor_invalid" {
		t.Fatalf("wanted empty numbers/static descriptor refusal; got %+v, %v", v, e)
	}
}

// All720 flag permutations preserve the mapping, including FD3/255 endpoints.
// This proves argument syntax only, never descriptor existence/type/custody.
func TestDescriptorPermutations(t *testing.T) {
	base := fdArgs()
	want := DescriptorNumbers{3, 4, 5, 6, 7, 255}
	count := 0
	var visit func([]int, int)
	visit = func(order []int, pos int) {
		if pos == len(order) {
			args := make([]string, 0, 12)
			for _, i := range order {
				args = append(args, base[2*i], base[2*i+1])
			}
			before := append([]string(nil), args...)
			v, e := ParseDescriptorArgs(args)
			if e != nil || v != want || !reflect.DeepEqual(args, before) {
				t.Fatal(v, e)
			}
			count++
			return
		}
		for i := pos; i < len(order); i++ {
			order[pos], order[i] = order[i], order[pos]
			visit(order, pos+1)
			order[pos], order[i] = order[i], order[pos]
		}
	}
	visit([]int{0, 1, 2, 3, 4, 5}, 0)
	if count != 720 {
		t.Fatal(count)
	}
}

// Every flag/value slot rejects illegal decimal neighbors, aliasing, omission
// and repetition; extras/operands/native tails cannot be ignored or defaulted.
func TestDescriptorRefusals(t *testing.T) {
	if _, e := ParseDescriptorArgs(fdArgs()); e != nil {
		t.Fatal(e)
	}
	for slot := 0; slot < 6; slot++ {
		for _, bad := range []string{"", "0", "1", "2", "256", "999", "1000", "03", "00", "+3", "-3", "3.0", "3e0", " 3", "3 ", "3\n", "３", "٣", "0x3", "private/path/secret-marker"} {
			args := fdArgs()
			args[slot*2+1] = bad
			fdBad(t, args)
		}
		args := fdArgs()
		args[slot*2] = "--unknown"
		fdBad(t, args)
		args = fdArgs()
		args[slot*2] = args[((slot+1)%6)*2]
		fdBad(t, args)
		args = fdArgs()
		args[slot*2+1] = args[((slot+1)%6)*2+1]
		fdBad(t, args)
		args = fdArgs()
		args = append(args[:slot*2], args[slot*2+2:]...)
		fdBad(t, args)
	}
	for _, args := range [][]string{nil, {}, {"member"}, append(fdArgs(), "operand"), append(fdArgs(), "--"), append(fdArgs(), "--native", "tail"), append(fdArgs(), "--request-fd", "8"), append([]string{"member"}, fdArgs()...), {"--request-fd=3", "--authorization-fd=4", "--objects-fd=5", "--state-fd=6", "--response-fd=7", "--controller-fd=8"}} {
		fdBad(t, args)
	}
}
