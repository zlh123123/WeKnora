# Knowledge MRI Evaluation

Run:

```bash
python3 scripts/knowledge_mri_eval.py --seed 20260829 --concepts 500 --attempts 6
```

The script uses a fixed seed and a synthetic latent mastery process. It reports status counts and Brier scores for an exposure-only baseline versus verified mastery multiplied by confidence. Synthetic results are a pilot sanity check, not evidence from a user study. Real mapping and quiz-grounding samples should be appended when a stable evaluation KB is available.
