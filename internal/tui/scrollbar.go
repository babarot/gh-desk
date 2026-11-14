package tui

// track is where a pane's view scrolls, for a scrollbar on its frame: the
// body's rows from row on, room of them, showing the lines from first out
// of total. The zero value has no scrollbar.
type track struct{ row, room, first, total int }

// thumbAt places a scrollbar's thumb on a track of room rows, for a view of
// room lines starting at first out of total, as the rows [from, to). It
// touches an end of the track only when the view is at that end, so that a
// thumb at an end says nothing is past it.
func thumbAt(first, room, total int) (from, to int) {
	size := max(1, (room*room+total/2)/total)
	span, last := room-size, total-room
	from = (first*span + last/2) / last
	if first > 0 && from == 0 && span > 1 {
		from = 1
	}
	if first < last && from == span && span > 1 {
		from = span - 1
	}
	return from, from + size
}

// thumb says whether row i of a pane's body is on the thumb of its
// scrollbar; there is none when all its lines show.
func (t track) thumb(i int) bool {
	if t.room <= 0 || t.total <= t.room {
		return false
	}
	from, to := thumbAt(t.first, t.room, t.total)
	return i >= t.row+from && i < t.row+to
}
