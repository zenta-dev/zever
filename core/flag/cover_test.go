package flag

import (
)

// TestCoverTypedErrorStrings exercises every typed Error method so the
// message constructors stay covered. It asserts prefixes, not exact
// text, per the no-brittle-assertions rule.
