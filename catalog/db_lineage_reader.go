package catalog

import (
	"fmt"

	"github.com/treeverse/lakefs/db"
)

type DBLineageReader struct {
	tx           db.Tx
	branchID     int64
	EOF          bool
	commitID     CommitID
	readers      []*DBBranchReader
	nextRow      []*DBReaderEntry
	returnedRows int
	bufSize      int
	after        string
}

func NewDBLineageReader(tx db.Tx, branchID int64, commitID CommitID, bufSize int, after string) *DBLineageReader {
	return &DBLineageReader{
		tx:       tx,
		branchID: branchID,
		commitID: commitID,
		bufSize:  bufSize,
		after:    after,
	}
}

func (r *DBLineageReader) ensureReaders() error {
	if r.readers != nil {
		return nil
	}
	lineage, err := getLineage(r.tx, r.branchID, r.commitID)
	if err != nil {
		return fmt.Errorf("error getting lineage: %w", err)
	}
	r.readers = make([]*DBBranchReader, len(lineage)+1)
	r.readers[0] = NewDBBranchReader(r.tx, r.branchID, r.commitID, r.bufSize, r.after)
	for i, bl := range lineage {
		r.readers[i+1] = NewDBBranchReader(r.tx, bl.BranchID, bl.CommitID, r.bufSize, r.after)
	}
	r.nextRow = make([]*DBReaderEntry, len(r.readers))
	for i, reader := range r.readers {
		if reader.Next() {
			r.nextRow[i] = reader.Value()
		} else if reader.Err() != nil {
			return fmt.Errorf("getting entry from branch ID %d: %w", reader.branchID, reader.Err())
		}
	}
	return nil
}

func (r *DBLineageReader) Next() (*DBReaderEntry, error) {
	if r.EOF {
		return nil, nil
	}
	if err := r.ensureReaders(); err != nil {
		return nil, err
	}

	// indirection array, to skip lineage branches that reached end
	nonNilNextRow := make([]int, 0, len(r.nextRow))
	for i, ent := range r.nextRow {
		if ent != nil {
			nonNilNextRow = append(nonNilNextRow, i)
		}
	}
	if len(nonNilNextRow) == 0 {
		r.EOF = true
		return nil, nil
	}

	// find lowest Path
	selectedEntry := r.nextRow[nonNilNextRow[0]]
	for i := 1; i < len(nonNilNextRow); i++ {
		if selectedEntry.Path > r.nextRow[nonNilNextRow[i]].Path {
			selectedEntry = r.nextRow[nonNilNextRow[i]]
		}
	}
	r.returnedRows++

	// advance next row for all branches that have this Path
	for i := 0; i < len(nonNilNextRow); i++ {
		branchIdx := nonNilNextRow[i]
		if r.nextRow[branchIdx].Path == selectedEntry.Path {
			var ent *DBReaderEntry
			reader := r.readers[branchIdx]
			if reader.Next() {
				ent = reader.Value()
			} else if reader.Err() != nil {
				return nil, fmt.Errorf("getting entry on branch: %w", reader.Err())
			}
			r.nextRow[branchIdx] = ent
		}
	}
	return selectedEntry, nil
}
