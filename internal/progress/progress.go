package progress

type Writer struct {
	Total int64
	Fn    func(done, total int64)
	done  int64
	last  int64
}

func (w *Writer) Write(b []byte) (int, error) {
	w.done += int64(len(b))
	if w.Fn != nil && (w.done-w.last > 512<<10 || w.done == w.Total) {
		w.last = w.done
		w.Fn(w.done, w.Total)
	}
	return len(b), nil
}
