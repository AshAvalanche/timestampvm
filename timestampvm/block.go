// (c) 2019-2020, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package timestampvm

import (
	"errors"
	"fmt"
	"time"

	"github.com/ava-labs/avalanchego/ids"
	"github.com/ava-labs/avalanchego/snow/choices"
	"github.com/ava-labs/avalanchego/snow/consensus/snowman"
	"github.com/ava-labs/avalanchego/utils/hashing"
)

var (
	errTimestampTooEarly = errors.New("block's timestamp is earlier than its parent's timestamp")
	errDatabaseGet       = errors.New("error while retrieving data from database")
	errTimestampTooLate  = errors.New("block's timestamp is more than 1 hour ahead of local time")
	errBlockNil          = errors.New("block is nil")

	_ Block = &TimeBlock{}
)

type Block interface {
	snowman.Block
	Initialize(bytes []byte, status choices.Status, vm *VM)
	Data() [dataLen]byte
}

// Block is a block on the chain.
// Each block contains:
// 1) A piece of data (a string)
// 2) A timestamp
type TimeBlock struct {
	PrntID ids.ID        `serialize:"true" json:"parentID"`  // parent's ID
	Hght   uint64        `serialize:"true" json:"height"`    // This block's height. The genesis block is at height 0.
	Tmstmp int64         `serialize:"true" json:"timestamp"` // Time this block was proposed at
	Dt     [dataLen]byte `serialize:"true" json:"data"`      // Arbitrary data

	id     ids.ID
	bytes  []byte
	status choices.Status
	vm     *VM
}

// Verify returns nil iff this block is valid.
// To be valid, it must be that:
// b.parent.Timestamp < b.Timestamp <= [local time] + 1 hour
func (b *TimeBlock) Verify() error {
	if b == nil {
		return errBlockNil
	}

	// Get [b]'s parent
	parentID := b.Parent()
	parent, err := b.vm.GetBlock(parentID)
	if err != nil {
		return errDatabaseGet
	}

	if expectedHeight := parent.Height() + 1; expectedHeight != b.Hght {
		return fmt.Errorf(
			"expected block to have height %d, but found %d",
			expectedHeight,
			b.Hght,
		)
	}

	// Ensure [b]'s timestamp is after its parent's timestamp.
	if b.Timestamp().Unix() < parent.Timestamp().Unix() {
		return errTimestampTooEarly
	}

	// Ensure [b]'s timestamp is not more than an hour
	// ahead of this node's time
	if b.Timestamp().Unix() >= time.Now().Add(time.Hour).Unix() {
		return errTimestampTooLate
	}

	b.vm.verifiedBlocks[b.id] = b

	return nil
}

// Initialize sets [b.bytes] to [bytes], sets [b.id] to hash([b.bytes])
// Checks if [b]'s status is already stored in state. If so, [b] gets that status.
// Otherwise [b]'s status is Unknown.
func (b *TimeBlock) Initialize(bytes []byte, status choices.Status, vm *VM) {
	b.vm = vm
	b.bytes = bytes
	b.id = hashing.ComputeHash256Array(b.bytes)
	b.status = status
}

// Accept sets this block's status to Accepted and sets lastAccepted to this
// block's ID and saves this info to b.vm.DB
func (b *TimeBlock) Accept() error {
	b.SetStatus(choices.Accepted) // Change state of this block
	blkID := b.ID()

	// Persist data
	if err := b.vm.state.PutBlock(b); err != nil {
		return err
	}

	if err := b.vm.state.SetLastAccepted(blkID); err != nil {
		return err
	}

	delete(b.vm.verifiedBlocks, b.ID())
	return b.vm.state.Commit()
}

// Reject sets this block's status to Rejected and saves the status in state
// Recall that b.vm.DB.Commit() must be called to persist to the DB
func (b *TimeBlock) Reject() error {
	b.SetStatus(choices.Rejected)
	if err := b.vm.state.PutBlock(b); err != nil {
		return err
	}
	delete(b.vm.verifiedBlocks, b.ID())
	return b.vm.state.Commit()
}

// ID returns the ID of this block
func (b *TimeBlock) ID() ids.ID { return b.id }

// ParentID returns [b]'s parent's ID
func (b *TimeBlock) Parent() ids.ID { return b.PrntID }

// Height returns this block's height. The genesis block has height 0.
func (b *TimeBlock) Height() uint64 { return b.Hght }

// Timestamp returns this block's time. The genesis block has time 0.
func (b *TimeBlock) Timestamp() time.Time { return time.Unix(b.Tmstmp, 0) }

// Status returns the status of this block
func (b *TimeBlock) Status() choices.Status { return b.status }

// Bytes returns the byte repr. of this block
func (b *TimeBlock) Bytes() []byte { return b.bytes }

// Data returns the data of this block
func (b *TimeBlock) Data() [dataLen]byte { return b.Dt }

// SetStatus sets the status of this block
func (b *TimeBlock) SetStatus(status choices.Status) { b.status = status }

func newTimeBlock(parentID ids.ID, height uint64, data [dataLen]byte, timestamp time.Time) *TimeBlock {
	// Create our new block
	return &TimeBlock{
		PrntID: parentID,
		Hght:   height,
		Tmstmp: timestamp.Unix(),
		Dt:     data,
	}
}
