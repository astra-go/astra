#!/usr/bin/env python3
"""Convert `go test -bench` output into a self-contained HTML comparison report.

Usage:
    go test -bench='^BenchmarkVs_' -benchmem -count=6 -benchtime=2s ./benchmarks/ \\
        > bench.txt
    python3 scripts/bench-to-html.py bench.txt            # writes report.html
    python3 scripts/bench-to-html.py bench.txt out.html   # custom output path
    python3 scripts/bench-to-html.py --badges bench.txt   # also write badge JSON
    python3 scripts/bench-to-html.py --platform "linux/amd64 · CI" bench.txt
"""
from __future__ import annotations

import argparse
import json
import os
import re
import sys
from collections import defaultdict
from datetime import datetime, timezone

# ─── parsing ──────────────────────────────────────────────────────────────────

LINE = re.compile(
    r"^Benchmark(Vs_)?(?P<framework>\w+?)_(?P<scenario>\w+)-\d+\s+"
    r"\d+\s+(?P<nsop>[\d.]+)\s+ns/op"
    r"(?:\s+(?P<bop>[\d.]+)\s+B/op)?"
    r"(?:\s+(?P<allocs>[\d.]+)\s+allocs/op)?"
)

def parse(path: str) -> dict[str, dict[str, list[dict]]]:
    """Return {scenario: {framework: [{nsop, bop, allocs}]}}"""
    data: dict[str, dict[str, list[dict]]] = defaultdict(lambda: defaultdict(list))
    with open(path) as f:
        for line in f:
            m = LINE.match(line.strip())
            if not m:
                continue
            fw = m.group("framework")
            sc = m.group("scenario")
            data[sc][fw].append({
                "nsop":   float(m.group("nsop")),
                "bop":    float(m.group("bop") or 0),
                "allocs": float(m.group("allocs") or 0),
            })
    return data

def avg(rows: list[dict], key: str) -> float:
    vals = [r[key] for r in rows]
    return sum(vals) / len(vals) if vals else 0.0


# ─── HTML generation ──────────────────────────────────────────────────────────

SCENARIO_LABELS = {
    "Baseline":  "Baseline — GET /ping → 204",
    "StaticJSON": "Static route — GET /health → JSON",
    "ParamJSON":  "Param route — GET /users/:id → JSON",
    "PostBind":   "POST bind — POST /users (JSON body) → JSON",
    "Mw3JSON":    "3-middleware stack → JSON",
}

FRAMEWORK_ORDER = ["Astra", "Gin", "Echo", "Fiber"]

FIBER_NOTE = (
    "* Fiber uses <code>app.Test()</code> (fasthttp↔net/http adapter) which adds "
    "~4 µs of serialisation overhead per call. Real-network Fiber performance is "
    "significantly higher."
)


def _color(ratio: float) -> str:
    """Green for ratio ≤ 1.0 (best), yellow around 1.2, red at 1.5+."""
    if ratio <= 1.0:
        return "#22c55e"   # green-500
    if ratio <= 1.1:
        return "#84cc16"   # lime-500
    if ratio <= 1.25:
        return "#eab308"   # yellow-500
    if ratio <= 1.5:
        return "#f97316"   # orange-500
    return "#ef4444"       # red-500


def _bar(ratio: float, width_px: int = 120) -> str:
    pct = min(ratio, 3.0) / 3.0 * width_px
    color = _color(ratio)
    return (
        f'<div style="display:inline-block;vertical-align:middle;'
        f'width:{width_px}px;background:#e5e7eb;border-radius:3px;margin-left:6px">'
        f'<div style="width:{pct:.1f}px;height:10px;background:{color};'
        f'border-radius:3px"></div></div>'
    )


def build_html(data: dict, generated_at: str, platform: str = "") -> str:
    # Aggregate: {scenario: {framework: {nsop, bop, allocs}}}
    agg: dict[str, dict[str, dict]] = {}
    for sc, fw_rows in data.items():
        agg[sc] = {}
        for fw, rows in fw_rows.items():
            agg[sc][fw] = {
                "nsop":   avg(rows, "nsop"),
                "bop":    avg(rows, "bop"),
                "allocs": avg(rows, "allocs"),
            }

    scenario_order = list(SCENARIO_LABELS.keys())
    # Fall back to whatever order we found if canonical labels don't match
    found = [s for s in scenario_order if s in agg] + \
            [s for s in agg if s not in scenario_order]

    frameworks = [f for f in FRAMEWORK_ORDER if any(f in agg[s] for s in agg)]
    if not frameworks:
        frameworks = sorted({fw for s in agg.values() for fw in s})

    rows_html = ""
    for sc in found:
        if sc not in agg:
            continue
        fw_data = agg[sc]
        # Best ns/op among all frameworks present
        best_ns = min((fw_data[f]["nsop"] for f in frameworks if f in fw_data), default=1)
        label = SCENARIO_LABELS.get(sc, sc)
        rows_html += f'<tr><td class="sc">{label}</td>'
        for fw in frameworks:
            if fw not in fw_data:
                rows_html += '<td class="num">—</td><td class="num">—</td>'
                continue
            d = fw_data[fw]
            ratio = d["nsop"] / best_ns if best_ns else 1.0
            color = _color(ratio)
            ns_fmt = f'{d["nsop"]:,.0f}'
            bop_fmt = f'{d["bop"]:,.0f}'
            allocs_fmt = f'{d["allocs"]:,.0f}'
            bar = _bar(ratio)
            badge = ' <span class="best">best</span>' if ratio <= 1.01 else ""
            rows_html += (
                f'<td class="num" style="color:{color}">'
                f'{ns_fmt}{bar}{badge}</td>'
                f'<td class="num">{bop_fmt} / {allocs_fmt}</td>'
            )
        rows_html += "</tr>\n"

    # Header row
    header_fw = "".join(
        f'<th colspan="2">{fw}{"*" if fw == "Fiber" else ""}</th>'
        for fw in frameworks
    )
    subheader_fw = "".join(
        '<th>ns/op</th><th>B/op / allocs</th>'
        for _ in frameworks
    )

    platform_note = f" · {platform}" if platform else ""
    return f"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Astra Benchmark — Framework Comparison</title>
<style>
  body{{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;
        background:#0f172a;color:#e2e8f0;margin:0;padding:2rem}}
  h1{{font-size:1.6rem;font-weight:700;margin-bottom:.25rem}}
  .subtitle{{color:#94a3b8;font-size:.85rem;margin-bottom:2rem}}
  table{{border-collapse:collapse;width:100%;font-size:.85rem}}
  th{{background:#1e293b;padding:.6rem .8rem;text-align:center;
      border:1px solid #334155;font-weight:600;color:#93c5fd}}
  td{{padding:.5rem .8rem;border:1px solid #1e293b;vertical-align:middle}}
  td.sc{{color:#e2e8f0;min-width:220px;font-size:.8rem}}
  td.num{{text-align:right;font-variant-numeric:tabular-nums;white-space:nowrap}}
  tr:nth-child(even){{background:#0f172a}}
  tr:nth-child(odd){{background:#111827}}
  tr:hover{{background:#1e293b}}
  .best{{background:#166534;color:#bbf7d0;font-size:.7rem;padding:1px 5px;
          border-radius:3px;margin-left:4px;vertical-align:middle}}
  .note{{color:#94a3b8;font-size:.78rem;margin-top:1.2rem;line-height:1.5}}
  .legend{{display:flex;gap:1rem;margin-bottom:1rem;flex-wrap:wrap}}
  .leg{{display:flex;align-items:center;gap:.3rem;font-size:.78rem}}
  .dot{{width:10px;height:10px;border-radius:50%}}
  a{{color:#60a5fa}}
  .platform-badge{{display:inline-block;background:#1e3a5f;color:#93c5fd;
                   font-size:.75rem;padding:2px 8px;border-radius:4px;
                   margin-left:.5rem;vertical-align:middle}}
</style>
</head>
<body>
<h1>Astra Framework Benchmark <span class="platform-badge">{platform or "local"}</span></h1>
<p class="subtitle">Generated {generated_at}{platform_note} · <a href="results.txt">raw results</a></p>

<div class="legend">
  <div class="leg"><div class="dot" style="background:#22c55e"></div>Best / ≤ 1%</div>
  <div class="leg"><div class="dot" style="background:#84cc16"></div>≤ 10% slower</div>
  <div class="leg"><div class="dot" style="background:#eab308"></div>≤ 25% slower</div>
  <div class="leg"><div class="dot" style="background:#f97316"></div>≤ 50% slower</div>
  <div class="leg"><div class="dot" style="background:#ef4444"></div>&gt; 50% slower</div>
</div>

<table>
<thead>
  <tr><th rowspan="2">Scenario</th>{header_fw}</tr>
  <tr>{subheader_fw}</tr>
</thead>
<tbody>
{rows_html}</tbody>
</table>

<p class="note">{FIBER_NOTE}</p>
<p class="note">
  All benchmarks run via <code>httptest.ResponseRecorder</code> (in-process, no network).
  Lower is better. Counts are geometric mean of 6 runs × 2 s each.
  Environment: {platform or "local machine"}.
  Shared CI runners have ±15% noise; treat numbers as relative comparisons, not absolutes.
</p>
</body>
</html>
"""


# ─── badge JSON (shields.io endpoint format) ──────────────────────────────────

def write_badges(data: dict, out_dir: str) -> None:
    agg: dict[str, dict[str, dict]] = {}
    for sc, fw_rows in data.items():
        agg[sc] = {}
        for fw, rows in fw_rows.items():
            agg[sc][fw] = {"nsop": avg(rows, "nsop"), "bop": avg(rows, "bop")}

    sc = "StaticJSON"
    if sc not in agg or "Astra" not in agg[sc]:
        return

    nsop = agg[sc]["Astra"]["nsop"]
    rps = 1e9 / nsop if nsop else 0
    if rps >= 1_000_000:
        rps_str = f"{rps/1_000_000:.1f}M req/s"
    elif rps >= 1_000:
        rps_str = f"{rps/1_000:.0f}K req/s"
    else:
        rps_str = f"{rps:.0f} req/s"

    bop = agg[sc]["Astra"]["bop"]
    mem_str = f"{bop:.0f} B/op (static JSON)"

    for name, label, message, color in [
        ("badge-rps.json", "Astra RPS", rps_str, "brightgreen"),
        ("badge-mem.json", "Astra mem", mem_str, "blue"),
    ]:
        payload = {"schemaVersion": 1, "label": label,
                   "message": message, "color": color}
        with open(os.path.join(out_dir, name), "w") as f:
            json.dump(payload, f, indent=2)
        print(f"wrote {os.path.join(out_dir, name)}")


# ─── main ─────────────────────────────────────────────────────────────────────

def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", help="go test -bench output file")
    parser.add_argument("output", nargs="?", default="report.html",
                        help="output HTML path (default: report.html)")
    parser.add_argument("--badges", action="store_true",
                        help="also write badge JSON files alongside the output")
    parser.add_argument("--platform", default="",
                        help="platform label shown in report subtitle (e.g. 'linux/amd64 · CI')")
    args = parser.parse_args()

    data = parse(args.input)
    if not data:
        print("No BenchmarkVs_ entries found in input.", file=sys.stderr)
        sys.exit(1)

    now = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    html = build_html(data, now, platform=args.platform)

    with open(args.output, "w") as f:
        f.write(html)
    print(f"wrote {args.output}")

    if args.badges:
        write_badges(data, os.path.dirname(os.path.abspath(args.output)) or ".")


if __name__ == "__main__":
    main()
