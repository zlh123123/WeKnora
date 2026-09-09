#!/usr/bin/env python3
"""Reproducible Knowledge MRI pilot evaluation (stdlib only)."""
import argparse, json, random, statistics

def simulate(seed, concepts, attempts):
    rng = random.Random(seed)
    rows = []
    for concept in range(concepts):
        latent = rng.random()
        answers = [rng.random() < latent for _ in range(attempts)]
        exposure = sum(rng.randint(0, 3) for _ in range(2))
        recent = answers[-4:]
        verified = sum(recent) / len(recent)
        confidence = min(len(recent) / 4, 1)
        status = "uncertain" if len(recent) < 2 else ("verified_strong" if verified >= .8 else "verified_weak" if verified <= .4 else "uncertain")
        rows.append({"concept": concept, "latent_mastery": latent, "exposure": exposure, "verified_mastery": verified, "confidence": confidence, "status": status, "next_correct": rng.random() < latent})
    return rows

def brier(rows, key):
    return statistics.mean((float(r[key]) - float(r["next_correct"])) ** 2 for r in rows)

def main():
    p = argparse.ArgumentParser()
    p.add_argument("--seed", type=int, default=20260829)
    p.add_argument("--concepts", type=int, default=500)
    p.add_argument("--attempts", type=int, default=6)
    p.add_argument("--output", default="docs/knowledge_mri/evaluation.json")
    a = p.parse_args()
    rows = simulate(a.seed, a.concepts, a.attempts)
    for r in rows:
        r["exposure_score"] = min(r["exposure"] / 6, 1)
        r["mastery_score"] = r["verified_mastery"] * r["confidence"]
    result = {"seed": a.seed, "concepts": a.concepts, "attempts": a.attempts,
              "status_counts": {s: sum(r["status"] == s for r in rows) for s in ("verified_strong", "verified_weak", "uncertain")},
              "brier_exposure_only": brier(rows, "exposure_score"),
              "brier_verified_mastery_confidence": brier(rows, "mastery_score"),
              "note": "Synthetic latent-mastery pilot; not a user study."}
    with open(a.output, "w", encoding="utf-8") as f: json.dump(result, f, ensure_ascii=False, indent=2)
    print(json.dumps(result, ensure_ascii=False, indent=2))

if __name__ == "__main__": main()
