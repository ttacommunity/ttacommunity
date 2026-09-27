import numpy as np
import pytest
from tta_ratings_engine.models.whr.solver import solve_tridiagonal

def test_thomas_solver_identity():
    # 3x3 diagonal system
    diag = np.array([2.0, 2.0, 2.0])
    off = np.array([-1.0, -1.0])
    rhs = np.array([1.0, 0.0, 1.0])

    # A = [[2, -1, 0], [-1, 2, -1], [0, -1, 2]]
    # A * [1, 1, 1] = [1, 0, 1] -> solution should be [1, 1, 1]
    sol = solve_tridiagonal(diag, off, rhs)
    np.testing.assert_allclose(sol, [1.0, 1.0, 1.0], atol=1e-5)
