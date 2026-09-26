"""Tridiagonal linear solver using the Thomas algorithm for Whole-History Rating."""
import numpy as np

def solve_tridiagonal(diag: np.ndarray, off_diag: np.ndarray, rhs: np.ndarray) -> np.ndarray:
    """Solve symmetric positive-definite tridiagonal linear system A x = rhs.
    
    A is symmetric tridiagonal:
      diag: main diagonal (length n)
      off_diag: sub- and super-diagonal (length n - 1)
      rhs: right hand side vector (length n)
    """
    n = len(rhs)
    if n == 0:
        return np.array([], dtype=float)
    if n == 1:
        return np.array([rhs[0] / diag[0]], dtype=float)

    c_prime = np.zeros(n - 1, dtype=float)
    d_prime = np.zeros(n, dtype=float)

    # Forward sweep
    c_prime[0] = off_diag[0] / diag[0]
    d_prime[0] = rhs[0] / diag[0]

    for i in range(1, n - 1):
        denom = diag[i] - off_diag[i - 1] * c_prime[i - 1]
        c_prime[i] = off_diag[i] / denom
        d_prime[i] = (rhs[i] - off_diag[i - 1] * d_prime[i - 1]) / denom

    denom = diag[n - 1] - off_diag[n - 2] * c_prime[n - 2]
    d_prime[n - 1] = (rhs[n - 1] - off_diag[n - 2] * d_prime[n - 2]) / denom

    # Back substitution
    x = np.zeros(n, dtype=float)
    x[n - 1] = d_prime[n - 1]
    for i in range(n - 2, -1, -1):
        x[i] = d_prime[i] - c_prime[i] * x[i + 1]

    return x
