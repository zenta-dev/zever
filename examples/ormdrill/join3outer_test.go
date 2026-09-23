package ormdrill

// ExampleDemoJoin3OuterMixed runs the homogeneous LEFT chain, the two mixed
// INNER/LEFT chains, and the RIGHT/FULL three-table builders.
func ExampleDemoJoin3OuterMixed() {
	runDemo(DemoJoin3OuterMixed)

	// Output:
	// == Round 6/7: three-table outer + mixed-kind joins
	//   LeftJoinOn3:      18 rows, B present 15, C present 7
	//   InnerLeftJoinOn3: 15 rows, C present 7
	//   LeftInnerJoinOn3: 10 rows, B present 7
	//   MixedJoinOn3:     15 rows (inner/left), C present 7
	//   MixedJoinOn3:     7 rows (left/right)
	//   RightJoinOn3:     7 rows
	//   FullJoinOn3:      18 rows
}
