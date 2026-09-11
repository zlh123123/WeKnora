#!/usr/bin/env python3
"""Read-only, allowlisted Knowledge MRI export and offline review aggregation."""
import argparse, collections, hashlib, json, random, subprocess, uuid
from pathlib import Path


def dump(path, data):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


def export(args):
    ids = [str(uuid.UUID(k)) for k in args.kb]
    quoted = ','.join("'%s'" % k for k in ids)
    # No profiles, attempts, users, model credentials, or messages are exported.
    query = f"""SELECT json_build_object(
      'pages',(SELECT coalesce(json_agg(x),'[]') FROM (SELECT id,knowledge_base_id,title,slug,summary,chunk_refs FROM wiki_pages WHERE knowledge_base_id IN ({quoted}) AND page_type='concept' AND status='published' AND deleted_at IS NULL ORDER BY id) x),
      'chunks',(SELECT coalesce(json_agg(x),'[]') FROM (SELECT c.id,c.knowledge_base_id,k.title AS document,c.content,c.is_enabled,c.chunk_type FROM chunks c JOIN knowledges k ON c.knowledge_id=k.id WHERE c.knowledge_base_id IN ({quoted}) AND c.deleted_at IS NULL AND k.deleted_at IS NULL ORDER BY c.id) x),
      'quizzes',(SELECT coalesce(json_agg(x),'[]') FROM (SELECT q.id,q.knowledge_base_id,i.title AS concept,q.question,q.options,q.correct_option,q.explanation,q.source_chunk_ids,q.source_hash,q.prompt_version,q.is_active,q.is_stale FROM quiz_items q LEFT JOIN learning_concept_identities i ON q.concept_key=i.concept_key AND q.knowledge_base_id=i.knowledge_base_id AND q.tenant_id=i.tenant_id WHERE q.knowledge_base_id IN ({quoted}) ORDER BY q.id) x));"""
    raw = subprocess.check_output(['docker','exec','-i',args.container,'sh','-c',
        'exec psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'], input=query, text=True)
    data = json.loads(raw)
    chunks = {c['id']: c for c in data['chunks']}
    aliases = {k: 'KB%d' % (i+1) for i,k in enumerate(ids)}
    stats, mappings = [], []
    rng = random.Random(args.seed)
    for kb in ids:
        pages = [p for p in data['pages'] if p['knowledge_base_id']==kb]
        pairs = [(p,cid) for p in pages for cid in (p['chunk_refs'] or [])]
        fanout = collections.Counter(cid for _,cid in pairs)
        valid = [(p,cid) for p,cid in pairs if cid in chunks and chunks[cid]['knowledge_base_id']==kb and chunks[cid]['is_enabled']]
        stats.append({'kb':aliases[kb], 'concepts':len(pages), 'with_refs':sum(bool(p['chunk_refs']) for p in pages),
            'reference_pairs':len(pairs), 'resolvable_enabled_pairs':len(valid), 'distinct_chunks':len(fanout),
            'fanout_histogram':dict(sorted(collections.Counter(fanout.values()).items())), 'max_fanout':max(fanout.values(),default=0)})
        # Stratify by KB; deterministic sample from all mappings, not only supported ones.
        for p,cid in rng.sample(pairs,min(args.per_kb,len(pairs))):
            c = chunks.get(cid,{})
            mappings.append({'id':'M%02d' % (len(mappings)+1), 'kb':aliases[kb], 'concept':p['title'],
                'concept_summary':p['summary'], 'document':c.get('document'), 'source':c.get('content',''),
                'source_sha256':hashlib.sha256(c.get('content','').encode()).hexdigest(), 'resolvable':cid in chunks})
    quizzes=[]
    for q in data['quizzes']:
        if not q['is_active'] or q['is_stale']: continue
        sources = [{'document':chunks[cid]['document'], 'content':chunks[cid]['content']} for cid in q['source_chunk_ids'] if cid in chunks]
        quizzes.append({'id':'Q%02d'%(len(quizzes)+1),'kb':aliases[q['knowledge_base_id']],
            **{k:q[k] for k in ['concept','question','options','correct_option','explanation','source_hash','prompt_version']}, 'sources':sources})
    result={'schema':1,'seed':args.seed,'sampling':f'{args.per_kb} mapping pairs per KB; census of active non-stale bank items',
        'scope':'Public RAG tutorial and d2l-zh computer-vision material only. No user learning records.',
        'stats':stats,'mappings':mappings,'quizzes':quizzes}
    dump(args.output,result)
    print(json.dumps({'stats':stats,'mapping_samples':len(mappings),'quiz_items':len(quizzes)},ensure_ascii=False,indent=2))


def report(args):
    data=json.loads(Path(args.input).read_text())
    review=json.loads(Path(args.review).read_text())
    required={'mappings':['supported'],'quizzes':['grounded','unique_answer','concept_relevant','nonduplicate']}
    out={'reviewer':review['reviewer'],'independent_human_review':False,'user_study':False,'stats':data['stats']}
    for kind,fields in required.items():
        expected={x['id'] for x in data[kind]}; rows=review[kind]
        if {r['id'] for r in rows} != expected or len(rows)!=len(expected): raise ValueError('Missing or duplicate review IDs: '+kind)
        for row in rows:
            if not row.get('reason'): raise ValueError('Review reason required')
            for f in fields:
                if row[f] not in ['yes','no','uncertain']: raise ValueError('Invalid review label')
        counts={f:dict(collections.Counter(r[f] for r in rows)) for f in fields}
        passed=sum(all(r[f]=='yes' for f in fields) for r in rows)
        out[kind]={'n':len(rows),'labels':counts,'all_criteria_pass':passed,'all_criteria_pass_rate':passed/len(rows) if rows else None}
    dump(args.output,out)
    print(json.dumps(out,ensure_ascii=False,indent=2))


def main():
    p=argparse.ArgumentParser(description=__doc__); sub=p.add_subparsers(dest='action',required=True)
    e=sub.add_parser('export');e.add_argument('--container',default='WeKnora-postgres');e.add_argument('--kb',action='append',required=True)
    e.add_argument('--seed',type=int,default=20260910);e.add_argument('--per-kb',type=int,default=15);e.add_argument('--output',required=True);e.set_defaults(func=export)
    r=sub.add_parser('report');r.add_argument('--input',required=True);r.add_argument('--review',required=True);r.add_argument('--output',required=True);r.set_defaults(func=report)
    a=p.parse_args();a.func(a)
if __name__=='__main__':main()
