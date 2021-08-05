package storage

import (
	"os"
	"testing"

	"github.com/google/pprof/profile"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/require"
)

func TestTreeStack(t *testing.T) {
	s := TreeStack{}
	s.Push(&TreeStackEntry{node: &TreeNode{Name: "a"}})
	s.Push(&TreeStackEntry{node: &TreeNode{Name: "b"}})

	require.Equal(t, 2, s.Size())

	e, hasMore := s.Pop()
	require.True(t, hasMore)
	require.Equal(t, "b", e.node.Name)

	require.Equal(t, 1, s.Size())

	e, hasMore = s.Pop()
	require.True(t, hasMore)
	require.Equal(t, "a", e.node.Name)

	require.Equal(t, 0, s.Size())

	e, hasMore = s.Pop()
	require.False(t, hasMore)
}

func TestLinesToTreeNodes(t *testing.T) {
	outerMost, innerMost := linesToTreeNodes([]profile.Line{
		{
			Function: &profile.Function{
				Name: "memcpy",
			},
		}, {
			Function: &profile.Function{
				Name: "printf",
			},
		}, {
			Function: &profile.Function{
				Name: "log",
			},
		},
	}, 2)

	require.Equal(t, &TreeNode{
		Name:     "log :0",
		FullName: "log :0",
		Cum:      2,
		Children: []*TreeNode{{
			Name:     "printf :0",
			FullName: "printf :0",
			Cum:      2,
			Children: []*TreeNode{{
				Name:     "memcpy :0",
				FullName: "memcpy :0",
				Cum:      2,
			}},
		}},
	}, outerMost)
	require.Equal(t, &TreeNode{
		Name:     "memcpy :0",
		FullName: "memcpy :0",
		Cum:      2,
	}, innerMost)
}

type fakeLocations struct {
	m map[uint64]*profile.Location
}

func (l *fakeLocations) GetLocationByID(id uint64) (*profile.Location, error) {
	return l.m[id], nil
}

func TestGenerateFlamegraph(t *testing.T) {
	pt := NewProfileTree()
	pt.Insert(makeSample(2, []uint64{2, 1}))
	pt.Insert(makeSample(1, []uint64{5, 3, 2, 1}))
	pt.Insert(makeSample(3, []uint64{4, 3, 2, 1}))

	l := &fakeLocations{m: map[uint64]*profile.Location{
		1: {Line: []profile.Line{{Function: &profile.Function{Name: "1"}}}},
		2: {Line: []profile.Line{{Function: &profile.Function{Name: "2"}}}},
		3: {Line: []profile.Line{{Function: &profile.Function{Name: "3"}}}},
		4: {Line: []profile.Line{{Function: &profile.Function{Name: "4"}}}},
		5: {Line: []profile.Line{{Function: &profile.Function{Name: "5"}}}},
	}}

	fg, err := generateFlamegraph(l, pt.Iterator())
	require.NoError(t, err)
	require.Equal(t, &TreeNode{
		Name: "root",
		Cum:  6,
		Children: []*TreeNode{{
			Name:     "1 :0",
			FullName: "1 :0",
			Cum:      6,
			Children: []*TreeNode{{
				Name:     "2 :0",
				FullName: "2 :0",
				Cum:      6,
				Children: []*TreeNode{{
					Name:     "3 :0",
					FullName: "3 :0",
					Cum:      4,
					Children: []*TreeNode{{
						Name:     "4 :0",
						FullName: "4 :0",
						Cum:      3,
					}, {
						Name:     "5 :0",
						FullName: "5 :0",
						Cum:      1,
					}},
				}},
			}},
		}},
	},
		fg)
}

func testGenerateFlamegraphFromProfileTree(t *testing.T) *TreeNode {
	f, err := os.Open("testdata/profile1.pb.gz")
	require.NoError(t, err)
	p1, err := profile.Parse(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	l := NewInMemoryProfileMetaStore()
	profileTree := ProfileTreeFromPprof(l, p1)

	fg, err := generateFlamegraph(l, profileTree.Iterator())
	require.NoError(t, err)

	return fg
}

func TestGenerateFlamegraphFromProfileTree(t *testing.T) {
	testGenerateFlamegraphFromProfileTree(t)
}

func testGenerateFlamegraphFromInstantProfile(t *testing.T) *TreeNode {
	f, err := os.Open("testdata/profile1.pb.gz")
	require.NoError(t, err)
	p1, err := profile.Parse(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	l := NewInMemoryProfileMetaStore()
	s, err := NewMemSeries(labels.Labels{{Name: "test_name", Value: "test_value"}}, 1)
	require.NoError(t, err)
	require.NoError(t, s.Append(ProfileFromPprof(l, p1)))

	it := s.Iterator()
	require.True(t, it.Next())
	require.NoError(t, it.Err())
	instantProfile := it.At()

	fg, err := generateFlamegraph(l, instantProfile.ProfileTree().Iterator())
	require.NoError(t, err)
	return fg
}

func TestGenerateFlamegraphFromInstantProfile(t *testing.T) {
	testGenerateFlamegraphFromInstantProfile(t)
}

func TestFlamegraphConsistency(t *testing.T) {
	require.Equal(t, testGenerateFlamegraphFromProfileTree(t), testGenerateFlamegraphFromInstantProfile(t))
}

func TestGenerateFlamegraphFromMergeProfile(t *testing.T) {
	testGenerateFlamegraphFromMergeProfile(t)
}

func testGenerateFlamegraphFromMergeProfile(t *testing.T) *TreeNode {
	f, err := os.Open("testdata/profile1.pb.gz")
	require.NoError(t, err)
	p1, err := profile.Parse(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	f, err = os.Open("testdata/profile2.pb.gz")
	require.NoError(t, err)
	p2, err := profile.Parse(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	l := NewInMemoryProfileMetaStore()
	prof1 := ProfileFromPprof(l, p1)
	prof2 := ProfileFromPprof(l, p2)

	m, err := NewMergeProfile(prof1, prof2)
	require.NoError(t, err)

	fg, err := generateFlamegraph(l, m.ProfileTree().Iterator())
	require.NoError(t, err)

	return fg
}

func TestControlGenerateFlamegraphFromMergeProfile(t *testing.T) {
	f, err := os.Open("testdata/merge.pb.gz")
	require.NoError(t, err)
	p1, err := profile.Parse(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	l := NewInMemoryProfileMetaStore()
	profileTree := ProfileTreeFromPprof(l, p1)

	fg, err := generateFlamegraph(l, profileTree.Iterator())
	require.NoError(t, err)

	mfg := testGenerateFlamegraphFromMergeProfile(t)
	require.Equal(t, fg, mfg)
}

func BenchmarkGenerateFlamegraph(b *testing.B) {
	f, err := os.Open("testdata/alloc_objects.pb.gz")
	require.NoError(b, err)
	p1, err := profile.Parse(f)
	require.NoError(b, err)
	require.NoError(b, f.Close())

	l := NewInMemoryProfileMetaStore()
	profileTree := ProfileTreeFromPprof(l, p1)

	b.ResetTimer()
	_, err = generateFlamegraph(l, profileTree.Iterator())
	require.NoError(b, err)
}

func TestAggregateByFunctionName(t *testing.T) {
	fg := &TreeNode{
		Name: "root",
		Cum:  6,
		Children: []*TreeNode{{
			Name:     "1 :0",
			FullName: "1 :0",
			Cum:      6,
			Children: []*TreeNode{{
				Name:     "2 :0",
				FullName: "2 :0",
				Cum:      6,
				Children: []*TreeNode{{
					Name:     "3 :0",
					FullName: "3 :0",
					Cum:      4,
					Children: []*TreeNode{{
						Name:     "4 :0",
						FullName: "4 :0",
						Cum:      3,
					}, {
						Name:     "5 :0",
						FullName: "5 :0",
						Cum:      1,
					}},
				}},
			},
				{
					Name:     "2 :0",
					FullName: "2 :0",
					Cum:      6,
					Children: []*TreeNode{{
						Name:     "3 :0",
						FullName: "3 :0",
						Cum:      4,
						Children: []*TreeNode{{
							Name:     "4 :0",
							FullName: "4 :0",
							Cum:      3,
						}, {
							Name:     "5 :0",
							FullName: "5 :0",
							Cum:      1,
						}},
					}},
				}},
		}},
	}

	afg := &TreeNode{
		Name: "root",
		Cum:  6,
		Children: []*TreeNode{{
			Name:     "1 :0",
			FullName: "1 :0",
			Cum:      6,
			Children: []*TreeNode{{
				Name:     "2 :0",
				FullName: "2 :0",
				Cum:      12,
				Children: []*TreeNode{
					{
						Name:     "3 :0",
						FullName: "3 :0",
						Cum:      8,
						Children: []*TreeNode{
							{
								Name:     "4 :0",
								FullName: "4 :0",
								Cum:      6,
							}, {
								Name:     "5 :0",
								FullName: "5 :0",
								Cum:      2,
							}},
					},
				},
			},
			},
		}},
	}

	require.Equal(t, aggregateByFunctionName(fg), afg)
}