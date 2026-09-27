"""Progress for a long engine run: done/total, elapsed and ETA, with a heartbeat so a hang shows.

Labels are step names such as model types and formats, never player names.
"""

import threading
import time


def _duration(seconds):
    seconds = round(seconds)
    h, rest = divmod(seconds, 3600)
    m, s = divmod(rest, 60)
    return f"{h}h{m:02d}m{s:02d}s" if h else f"{m}m{s:02d}s"


class Progress:
    """Counts steps towards a known total and logs at most once per `every` seconds.

    The first and last steps are always logged. log takes one line; clock
    returns seconds.
    """

    def __init__(self, total, log=print, clock=time.monotonic, every=30.0):
        self.total = total
        self.done = 0
        self._log = log
        self._clock = clock
        self._every = every
        self._start = clock()
        self._last_log = None
        self._current = "starting"
        self._lock = threading.Lock()

    def working_on(self, label):
        """Names the step in progress, for the heartbeat."""
        with self._lock:
            self._current = label

    def advance(self, label):
        """Marks one step done, logging it if due."""
        with self._lock:
            self.done += 1
            now = self._clock()
            due = self._last_log is None or now - self._last_log >= self._every
            if due or self.done >= self.total:
                self._emit(label, now)

    def beat(self):
        """Logs the step in progress if nothing has been logged for `every` seconds."""
        with self._lock:
            now = self._clock()
            since = now - (self._start if self._last_log is None else self._last_log)
            if since >= self._every:
                self._emit(f"still on {self._current}", now)

    def _emit(self, text, now):
        elapsed = now - self._start
        if self.done:
            eta = _duration(elapsed / self.done * max(0, self.total - self.done))
        else:
            eta = "?"
        self._log(
            f"[{self.done}/{self.total}] {text} | elapsed {_duration(elapsed)} | eta {eta}"
        )
        self._last_log = now


class Heartbeat:
    """A context manager that calls progress.beat() every `every` seconds on a daemon thread."""

    def __init__(self, progress, every=30.0):
        self._progress = progress
        self._every = every
        self._stop = threading.Event()
        self.thread = threading.Thread(target=self._run, daemon=True)

    def _run(self):
        while not self._stop.wait(self._every):
            self._progress.beat()

    def __enter__(self):
        self.thread.start()
        return self

    def __exit__(self, *exc):
        self._stop.set()
        self.thread.join()
