// One undo step per focus (OpenSpec wpi-property-editors, design D7): while the
// author stays in one property, only the first change takes an undo snapshot.
// Any other snapshot (a canvas action, undo, redo) ends that step, so the next
// change in the same property records a new one. A change made without focus
// records its own step.
export class EditSession {
	private open = false;
	private recorded = false;

	start() {
		this.open = true;
		this.recorded = false;
	}

	end() {
		this.open = false;
		this.recorded = false;
	}

	// Whether a property change now needs a snapshot; if so, the session counts it taken.
	takesSnapshot(): boolean {
		if (this.open && this.recorded) return false;
		this.recorded = this.open;
		return true;
	}

	// Another snapshot was taken or the stacks changed.
	interrupt() {
		this.recorded = false;
	}
}
