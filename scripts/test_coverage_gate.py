import unittest
from coverage_gate import bands, measure


class CoverageTests(unittest.TestCase):
    def test_exact_bands(self):
        self.assertEqual(bands('# comment\ninternal/store 85\ncmd/chartworks 70\n'), {'internal/store': 85, 'cmd/chartworks': 70})
        for value in ('', 'internal/store 85\ninternal/store 80', 'internal/store 101', '../store 80', 'internal/store zero'):
            with self.subTest(value=value), self.assertRaises(ValueError):
                bands(value)

    def test_weighted_statements_and_merged_instrumentation(self):
        profile = 'mode: atomic\nexample/internal/store/a.go:1.1,2.1 8 0\nexample/internal/store/a.go:1.1,2.1 8 1\nexample/internal/store/b.go:1.1,2.1 2 0\n'
        self.assertEqual(measure(profile, 'example', {'internal/store'}), {'internal/store': (8, 10)})

    def test_missing_package_cannot_pass(self):
        with self.assertRaises(ValueError):
            measure('mode: atomic\nexample/internal/a.go:1.1,2.1 1 1', 'example', {'internal/store'})

    def test_bad_profiles(self):
        for value in ('', 'mode: set\n', 'mode: atomic\nbad', 'mode: atomic\nexample/internal/store/a.go:1.1,2.1 -1 2', 'mode: atomic\nexample/internal/store/a.go:1.1,2.1 1 1\nexample/internal/store/a.go:1.1,2.1 2 1'):
            with self.subTest(value=value), self.assertRaises(ValueError):
                measure(value, 'example', {'internal/store'})
