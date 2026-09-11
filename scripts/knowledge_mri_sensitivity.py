#!/usr/bin/env python3
"""Frozen synthetic sensitivity study; no model, network or user data required."""
import argparse
import collections
import json
import random
from pathlib import Path


def run(seed=20260910, size=10000):
    rng = random.Random(seed)
    # Shared outcomes across exposure scenarios allow a paired comparison.
    rows = []
    for _ in range(size):
        p = rng.random()
        rows.append((p, [int(rng.random() < p) for _ in range(6)],
                     int(rng.random() < p), rng.random(), rng.gauss(0, .2)))
    results = []
    for scenario in ('independent', 'positively_correlated', 'negatively_correlated'):
        for count in (2, 4, 6):
            errors = collections.defaultdict(float)
            statuses = collections.Counter()
            false_strong = strong = 0
            for p, answers, outcome, independent, noise in rows:
                exposure = independent if scenario == 'independent' else min(1, max(0,
                    (p if scenario == 'positively_correlated' else 1-p) + noise))
                recent = answers[:count][-4:]
                mastery = sum(recent) / len(recent)
                confidence = len(recent) / 4
                predictions = {'constant_0.5': .5, 'exposure': exposure,
                               'mastery': mastery, 'mastery_times_confidence': mastery * confidence,
                               'laplace': (sum(recent)+1)/(len(recent)+2)}
                for key, value in predictions.items():
                    errors[key] += (value-outcome)**2
                status = 'strong' if mastery >= .8 else 'weak' if mastery <= .4 else 'uncertain'
                statuses[status] += 1
                strong += status == 'strong'
                false_strong += status == 'strong' and p < .8
            results.append({'exposure_scenario': scenario, 'attempts': count,
                            'brier': {k: v/size for k, v in errors.items()},
                            'status_counts': dict(statuses), 'strong_count': strong,
                            'strong_with_latent_below_0.8': false_strong,
                            'strong_with_latent_below_0.8_rate': false_strong/strong if strong else None})
    return {'seed': seed, 'samples': size, 'latent': 'Uniform(0,1)',
            'outcomes': 'independent Bernoulli(latent), fixed latent, no learning over time',
            'exposure': 'independent Uniform(0,1), or clip(p + N(0,0.2)), or clip(1-p + N(0,0.2))',
            'window': 4, 'note': 'Synthetic sensitivity only. Laplace is a comparison, not implemented product behavior. Confidence is sample coverage, not a probability.',
            'results': results}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--seed', type=int, default=20260910)
    parser.add_argument('--samples', type=int, default=10000)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    if args.samples < 1:
        parser.error('--samples must be positive')
    result = run(args.seed, args.samples)
    Path(args.output).write_text(json.dumps(result, indent=2)+'\n', encoding='utf-8')
    for row in result['results']:
        print(row['exposure_scenario'], row['attempts'],
              {k: round(v, 4) for k, v in row['brier'].items()})
