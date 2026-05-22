# Odd: 2-space indentation in Python, legal but unusual.

def add(a, b):
  return a + b


def sub(a, b):
  return a - b


def compute(values):
  total = 0
  for v in values:
    if v > 0:
      total += v
    else:
      total -= v
  return total


class Counter:
  def __init__(self):
    self.n = 0

  def inc(self):
    self.n += 1

  def get(self):
    return self.n
