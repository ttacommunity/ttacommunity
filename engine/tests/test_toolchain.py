"""The engine runs on the interpreter and numpy that its image ships."""

import sys

import numpy


def test_runs_on_python_3_12():
    assert sys.version_info[:2] == (3, 12), sys.version


def test_numpy_is_importable():
    assert numpy.array([1, 2]).sum() == 3
