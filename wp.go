package draupnir

func (wp *WorkerPool) worker() {
	for task := range wp.tasks {
		task()
		wp.wg.Done()
	}
}

// Submit adds a task to the pool and increments the waitgroup.
func (wp *WorkerPool) Submit(task func()) error {
	wp.wg.Add(1)
	wp.tasks <- task
	return nil
}

// Shutdown waits for all tasks to complete then closes the tasks channel.
func (wp *WorkerPool) Shutdown() {
	wp.wg.Wait()
	close(wp.tasks)
}
