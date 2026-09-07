import unittest
from coverage_gate import band_percentage, bands, measure, passes_band


class CoverageTests(unittest.TestCase):
    def test_exact_bands(self):
        self.assertEqual(bands('# comment\ninternal/store 85\ncmd/chartworks 70\n'), {'internal/store': 8500, 'cmd/chartworks': 7000})
        for value in ('', 'internal/store 85\ninternal/store 80', 'internal/store 101', '../store 80', 'internal/store zero'):
            with self.subTest(value=value), self.assertRaises(ValueError):
                bands(value)

    def test_decimal_bands_are_exact_basis_points(self):
        for text, expected in (('84.5', 8450), ('84.50', 8450), ('84.51', 8451), ('1.01', 101), ('100.00', 10000)):
            with self.subTest(text=text):
                self.assertEqual(bands('internal/store/postgres ' + text), {'internal/store/postgres': expected})
        for text in ('0.99', '100.01', '84.500', '84.', '.5', '8.45e1', '+84.5', '-84.5', 'NaN', 'Infinity'):
            with self.subTest(text=text), self.assertRaises(ValueError):
                bands('internal/store/postgres ' + text)
        self.assertEqual(band_percentage(8500), '85')
        self.assertEqual(band_percentage(8450), '84.5')
        self.assertEqual(band_percentage(8451), '84.51')

    def test_fractional_threshold_does_not_round_up_coverage(self):
        minimum = bands('internal/store/postgres 84.5')['internal/store/postgres']
        self.assertTrue(passes_band(169, 200, minimum))
        self.assertFalse(passes_band(168, 200, minimum))
        self.assertTrue(passes_band(1974, 2336, minimum))
        self.assertFalse(passes_band(1973, 2336, minimum))
        self.assertFalse(passes_band(84499, 100000, minimum))  # Displays as 84.50%.
        total = 10 ** 30
        self.assertFalse(passes_band(845 * total // 1000 - 1, total, minimum))
        self.assertFalse(passes_band(169, 200, bands('internal/store 85')['internal/store']))
        self.assertTrue(passes_band(170, 200, bands('internal/store 85')['internal/store']))

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
