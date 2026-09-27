import threading

import pytest

from tta_ratings_engine.progress import Heartbeat, Progress


class FakeClock:
    def __init__(self):
        self.now = 0.0

    def __call__(self):
        return self.now


@pytest.fixture
def clock():
    return FakeClock()


@pytest.fixture
def lines():
    return []


def progress(total, clock, lines, every=30.0):
    return Progress(total, log=lines.append, clock=clock, every=every)


def test_the_first_step_is_always_reported(clock, lines):
    p = progress(10, clock, lines)
    clock.now = 5

    p.advance("pairs")

    assert lines == ["[1/10] pairs | elapsed 0m05s | eta 0m45s"]


def test_steps_inside_the_interval_are_not_reported(clock, lines):
    p = progress(10, clock, lines)
    p.advance("a")
    clock.now = 10
    p.advance("b")

    assert len(lines) == 1


def test_a_step_after_the_interval_is_reported_with_an_eta(clock, lines):
    p = progress(10, clock, lines)
    p.advance("a")
    clock.now = 60
    p.advance("b")
    p.advance("c")

    assert lines[-1] == "[2/10] b | elapsed 1m00s | eta 4m00s"


def test_the_last_step_is_always_reported(clock, lines):
    p = progress(2, clock, lines)
    p.advance("a")
    clock.now = 1
    p.advance("b")

    assert lines[-1] == "[2/2] b | elapsed 0m01s | eta 0m00s"


def test_long_runs_show_hours(clock, lines):
    p = progress(2, clock, lines)
    clock.now = 3 * 3600 + 61

    p.advance("a")

    assert lines == ["[1/2] a | elapsed 3h01m01s | eta 3h01m01s"]


def test_a_heartbeat_says_what_is_still_running(clock, lines):
    p = progress(10, clock, lines)
    p.advance("a")
    p.working_on("walk-forward 3p")
    clock.now = 45

    p.beat()

    assert lines[-1] == "[1/10] still on walk-forward 3p | elapsed 0m45s | eta 6m45s"


def test_a_heartbeat_is_quiet_while_steps_are_being_reported(clock, lines):
    p = progress(10, clock, lines)
    p.advance("a")
    clock.now = 20

    p.beat()

    assert len(lines) == 1


def test_a_heartbeat_is_quiet_while_steps_keep_finishing(clock, lines):
    p = progress(10, clock, lines)
    p.advance("a")
    clock.now = 20
    p.advance("b")
    clock.now = 35

    p.beat()
    clock.now = 40
    p.advance("c")

    assert lines[-1] == "[3/10] c | elapsed 0m40s | eta 1m33s"
    assert not any("still on" in line for line in lines)


def test_a_heartbeat_before_any_step_has_no_eta(clock, lines):
    p = progress(10, clock, lines)
    p.working_on("pairs")
    clock.now = 31

    p.beat()

    assert lines == ["[0/10] still on pairs | elapsed 0m31s | eta ?"]


def test_the_heartbeat_thread_beats_until_stopped(lines):
    beaten = threading.Event()

    class Recorder:
        def beat(self):
            beaten.set()

    with Heartbeat(Recorder(), every=0.01) as hb:
        assert beaten.wait(timeout=5)
    assert not hb.thread.is_alive()


def test_the_eta_weighs_the_remaining_steps_by_their_cost(clock, lines):
    p = Progress(3, total_cost=10.0, log=lines.append, clock=clock)
    clock.now = 10

    p.advance("cheap", cost=1.0)

    assert lines == ["[1/3] cheap | elapsed 0m10s | eta 1m30s"]


def test_the_eta_adapts_to_how_fast_this_machine_is(clock, lines):
    p = Progress(3, total_cost=10.0, log=lines.append, clock=clock, every=0)
    clock.now = 10
    p.advance("a", cost=1.0)
    clock.now = 40

    p.advance("b", cost=5.0)

    assert lines[-1] == "[2/3] b | elapsed 0m40s | eta 0m27s"


def test_the_summary_gives_each_phases_steps_and_time(clock, lines):
    p = progress(3, clock, lines)
    clock.now = 5
    p.advance("m1", phase="models")
    clock.now = 15
    p.advance("m2", phase="models")
    clock.now = 20
    p.advance("c1", phase="calibration")
    lines.clear()

    p.summary()

    assert lines == [
        "models: 2 steps in 0m15s, 7.5s each",
        "calibration: 1 step in 0m05s, 5.0s each",
    ]
