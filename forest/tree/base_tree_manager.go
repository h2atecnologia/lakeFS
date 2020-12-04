package tree

import (
	"github.com/treeverse/lakefs/catalog/rocks"
	"github.com/treeverse/lakefs/forest/sstable"
)

type baseTreeManagerType struct {
	baseTree      TreeType
	partsForReuse TreeType
	baseIndex     int
	partManager   sstable.Manager
}

func (trees *TreesRepoType) newBaseTreeManager(treeID rocks.TreeID) (*baseTreeManagerType, error) {
	var baseParts TreeType
	var err error
	if treeID == "" {
		baseParts = make(TreeType, 0)
	} else {
		baseParts, err = trees.loadTreeIfNeeded(treeID)
		if err != nil {
			return nil, err
		}
	}
	return &baseTreeManagerType{
		baseTree:      baseParts,
		partsForReuse: make(TreeType, 0),
		partManager:   trees.PartManger,
	}, nil
}

func (bm *baseTreeManagerType) isEndOfBase() bool {
	return bm.baseIndex >= len(bm.baseTree)
}

func (bm *baseTreeManagerType) getBasePartForPath(path rocks.Path) (*pushBackEntryIterator, rocks.Path, error) {
	lenBaseTree := len(bm.baseTree)
	for ; bm.baseIndex < lenBaseTree &&
		bm.baseTree[bm.baseIndex].MaxPath < path; bm.baseIndex++ {
		bm.partsForReuse = append(bm.partsForReuse, bm.baseTree[bm.baseIndex])
	}
	if len(bm.baseTree) <= bm.baseIndex {
		return nil, minimalPath, InfoBaseTreeExhausted
	}
	p := bm.baseTree[bm.baseIndex]
	basePartIter, err := bm.partManager.NewSSTableIterator(p.PartName, minimalPath)
	bm.baseIndex++
	return newPushbackEntryIterator(basePartIter), p.MaxPath, err
}
func (bm *baseTreeManagerType) getPartsForReuse() *TreeType {
	if bm.baseIndex < len(bm.baseTree)-1 { // the apply loop did not reach the last parts of base, they will be added to reused
		bm.partsForReuse = append(bm.partsForReuse, bm.baseTree[bm.baseIndex:]...)
	}
	return &bm.partsForReuse
}

func (bm *baseTreeManagerType) isPathInNextPart(path rocks.Path) bool {
	if bm.isEndOfBase() {
		return true // last part of base is the active now. the new path wil be written to it
	} else {
		return path < bm.baseTree[bm.baseIndex].MaxPath
	}
}

func (bm *baseTreeManagerType) getBaseMaxPath() rocks.Path {
	return bm.baseTree[len(bm.baseTree)-1].MaxPath
}

func (bm *baseTreeManagerType) wasLastPartProcessed() bool {
	return len(bm.baseTree) == bm.baseIndex
}

func (bm *baseTreeManagerType) getLastPartIter() (*pushBackEntryIterator, error) {
	baseIter, _, err := bm.getBasePartForPath(bm.baseTree[len(bm.baseTree)-1].MaxPath)
	return baseIter, err
}