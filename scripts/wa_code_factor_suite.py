#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import os
from dataclasses import dataclass, field, replace
from pathlib import Path
import random
import string
import time
from typing import Any

import wa_code_param_probe as probe
from wa_code_random_device_experiment import DeviceProfile, build_device

ABPROP_URL = "https://y9yrsygcg6.execute-api.us-east-1.amazonaws.com/s/s?_=/v2/reg_onboard_abprop&"


@dataclass(frozen=True)
class FactorArm:
    group: str
    label: str
    device_label: str = "xiaomi-a11"
    transport: str = "requests"
    patches: tuple[str, ...] = ()
    sets: tuple[str, ...] = ()
    omits: tuple[str, ...] = ()
    envelope: str = "signed"
    preflight: str = ""
    stable_install: bool = False
    prefix: str = ""
    app_version: str = "2.26.23.71"


@dataclass
class SuiteArgs:
    proxy: str
    timeout: float
    show_fields: bool = False
    show_response: bool = False
    dry_run: bool = False
    variant: str = "current"
    set_param: list[str] = field(default_factory=list)
    omit: list[str] = field(default_factory=list)
    unsigned: bool = False
    empty_h: bool = False
    user_agent: str = ""
    device_display_id: str = ""
    device_ram: str = ""
    transport: str = "requests"


def classify(row: dict[str, Any]) -> str:
    if row.get("error"):
        return "transport_error"
    status = str(row.get("status") or "").lower()
    reason = str(row.get("reason") or "").lower()
    if status in {"sent", "ok"}:
        return "sent"
    if reason == "no_routes":
        return "no_routes"
    if reason == "blocked":
        return "blocked"
    if reason == "too_recent":
        return "too_recent"
    if reason == "bad_token":
        return "bad_token"
    if row.get("request_failed"):
        return "request_failed"
    if status == "fail":
        return "other_fail"
    return "unknown"


def random_colombia_phone_with_prefix(prefix: str) -> tuple[str, str]:
    return "57", prefix + "".join(random.choice(string.digits) for _ in range(7))


def next_phone(arm: FactorArm) -> tuple[str, str]:
    if arm.prefix:
        return random_colombia_phone_with_prefix(arm.prefix)
    return probe.random_colombia_phone()


def device_for_arm(arm: FactorArm) -> DeviceProfile:
    return build_device(arm.device_label)


def args_for_arm(base: argparse.Namespace, arm: FactorArm) -> SuiteArgs:
    device = device_for_arm(arm)
    user_agent = device.user_agent
    if arm.app_version != "2.26.23.71":
        user_agent = user_agent.replace("WhatsApp/2.26.23.71", "WhatsApp/" + arm.app_version, 1)
    return SuiteArgs(
        proxy=base.proxy,
        timeout=base.timeout,
        show_fields=base.show_fields,
        show_response=base.show_response,
        dry_run=base.dry_run,
        set_param=list(arm.sets),
        omit=list(arm.omits),
        unsigned=arm.envelope == "unsigned",
        empty_h=arm.envelope == "empty-h",
        user_agent=user_agent,
        device_display_id=device.display_id,
        device_ram=device.ram_gib,
        transport=arm.transport,
    )


def config_for_arm(arm: FactorArm, args: SuiteArgs) -> probe.ShapeConfig:
    config = probe.config_for_variant(args.variant)
    for patch in arm.patches:
        config = probe.apply_patch_name(config, patch)
    return probe.apply_cli_config_overrides(config, args)


def stable_material(base_material: probe.ProbeMaterial, fresh: probe.ProbeMaterial) -> probe.ProbeMaterial:
    return replace(
        fresh,
        fdid=base_material.fdid,
        expid=base_material.expid,
        expid_uuid=base_material.expid_uuid,
        access_session_id=base_material.access_session_id,
        access_session_id_uuid=base_material.access_session_id_uuid,
        id_raw=base_material.id_raw,
        backup_token_raw=base_material.backup_token_raw,
        authkey=base_material.authkey,
        key_bundle=base_material.key_bundle,
        advertising_id=base_material.advertising_id,
        created_at_unix=base_material.created_at_unix,
    )


def build_material(repo_root: Path, arm: FactorArm, stable_cache: dict[str, probe.ProbeMaterial]) -> probe.ProbeMaterial:
    cc, national = next_phone(arm)
    fresh = probe.new_probe_material(repo_root, cc, national)
    if not arm.stable_install:
        return fresh
    base = stable_cache.get(arm.label)
    if base is None:
        stable_cache[arm.label] = fresh
        return fresh
    return stable_material(base, fresh)


def build_abprop_params(material: probe.ProbeMaterial) -> list[probe.Param]:
    params: list[probe.Param] = []
    probe.add_param(params, "cc", material.cc)
    probe.add_param(params, "in", material.national)
    probe.add_param(params, "lg", "en")
    probe.add_param(params, "lc", "US")
    probe.add_param(params, "fdid", material.fdid)
    probe.add_param(params, "expid", material.expid)
    probe.add_param(params, "access_session_id", material.access_session_id)
    probe.add_param(params, "authkey", material.authkey)
    for key in ["e_ident", "e_keytype", "e_regid", "e_skey_id", "e_skey_val", "e_skey_sig"]:
        probe.add_param(params, key, material.key_bundle[key])
    return params


def post_abprop(material: probe.ProbeMaterial, args: SuiteArgs) -> dict[str, Any]:
    plain = probe.render_plain(build_abprop_params(material))
    envelope = probe.build_signed_wasafe_envelope(plain, material, "unsigned")
    headers = {
        "Content-Type": "application/x-www-form-urlencoded",
        "User-Agent": args.user_agent,
        "WaMsysRequest": "1",
        "X-Forwarded-Host": "v.whatsapp.net",
    }
    try:
        status, parsed = probe.post_form(args.transport, ABPROP_URL, headers, envelope.body, args.proxy, args.timeout)
        if not isinstance(parsed, dict):
            parsed = {"raw": parsed}
        return {
            "ab_http_status": status,
            "ab_status": parsed.get("status"),
            "ab_reason": parsed.get("reason") or parsed.get("failure_reason"),
            "ab_has_hash": bool(parsed.get("ab_hash")),
            "ab_has_exp_cfg": bool(parsed.get("exp_cfg")),
        }
    except Exception as exc:  # noqa: BLE001 - CLI probe must summarize failures.
        return {"ab_error": probe.sanitize_text(str(exc), args.proxy)}


def run_arm_once(repo_root: Path, base_args: argparse.Namespace, arm: FactorArm, stable_cache: dict[str, probe.ProbeMaterial]) -> dict[str, Any]:
    material = build_material(repo_root, arm, stable_cache)
    args = args_for_arm(base_args, arm)
    config = config_for_arm(arm, args)
    preflight_result: dict[str, Any] = {}
    if arm.preflight == "abprop":
        preflight_result = post_abprop(material, args)
    row = probe.post_code(material, config, args)
    row.update(preflight_result)
    row["group"] = arm.group
    row["label"] = arm.label
    row["transport"] = arm.transport
    row["device_label"] = arm.device_label
    row["app_version"] = arm.app_version
    row["outcome"] = classify(row)
    if arm.prefix:
        row["prefix"] = arm.prefix
    if arm.stable_install:
        row["stable_install"] = True
    return row


def rate(numerator: int, denominator: int) -> float | None:
    if denominator <= 0:
        return None
    return round(numerator / denominator, 4)


def summarize(rows: list[dict[str, Any]]) -> dict[str, Any]:
    labels = sorted({str(row.get("label") or "") for row in rows})
    summary: dict[str, Any] = {}
    for label in labels:
        group = [row for row in rows if row.get("label") == label]
        counts = {
            key: 0
            for key in [
                "sent",
                "no_routes",
                "blocked",
                "bad_token",
                "too_recent",
                "request_failed",
                "transport_error",
                "other_fail",
                "unknown",
            ]
        }
        for row in group:
            outcome = str(row.get("outcome") or "unknown")
            counts[outcome] = counts.get(outcome, 0) + 1
        total = len(group)
        target = counts["sent"] + counts["no_routes"]
        summary[label] = {
            "group": str(group[0].get("group") or "") if group else "",
            "total": total,
            **counts,
            "target_decisions": target,
            "sent_rate_on_target": rate(counts["sent"], target),
        }
    return summary


def markdown_table(summary: dict[str, Any]) -> str:
    headers = ["group", "label", "total", "sent", "no_routes", "blocked", "bad_token", "target", "sent/target"]
    lines = ["| " + " | ".join(headers) + " |", "|" + "---|" * len(headers)]
    for label, item in sorted(summary.items(), key=lambda pair: (str(pair[1].get("group")), pair[0])):
        values = [
            str(item.get("group")),
            label,
            str(item.get("total", 0)),
            str(item.get("sent", 0)),
            str(item.get("no_routes", 0)),
            str(item.get("blocked", 0)),
            str(item.get("bad_token", 0)),
            str(item.get("target_decisions", 0)),
            str(item.get("sent_rate_on_target")),
        ]
        lines.append("| " + " | ".join(values) + " |")
    return "\n".join(lines)


def factor_arms() -> list[FactorArm]:
    ghcr_patches = (
        "gpia-error-minus-two",
        "gpia-data-digest-ghcr",
        "gpia-source-ghcr",
        "gpia-json-no-slash-escape",
        "wamsys-ghcr",
    )
    wamsys_omits = ("gpia", "_ga", "_gi", "_gp", "_ge", "aid", "_gg")
    co_locale_sets = ("lg=es", "lc=CO")
    co_operator_patch = ("operator-co-732101",)
    combo_arms = []
    for prefix in ("314", "350"):
        combo_arms.extend(
            [
                FactorArm(
                    "combo",
                    f"combo-{prefix}-current",
                    patches=co_operator_patch,
                    sets=co_locale_sets,
                    prefix=prefix,
                ),
                FactorArm(
                    "combo",
                    f"combo-{prefix}-ghcr",
                    patches=(*co_operator_patch, *ghcr_patches),
                    sets=co_locale_sets,
                    prefix=prefix,
                ),
                FactorArm(
                    "combo",
                    f"combo-{prefix}-omit-wamsys",
                    patches=co_operator_patch,
                    sets=co_locale_sets,
                    omits=wamsys_omits,
                    prefix=prefix,
                ),
            ]
        )
    routing_base = FactorArm(
        "routing",
        "routing-baseline-350",
        patches=co_operator_patch,
        sets=co_locale_sets,
        prefix="350",
    )
    routing_arms = [
        routing_base,
        replace(routing_base, label="routing-us-locale", sets=()),
        replace(routing_base, label="routing-zero-operator", patches=()),
        replace(routing_base, label="routing-operator-omit", patches=("operator-omit",)),
        replace(routing_base, label="routing-simnum-one", sets=(*co_locale_sets, "simnum=1")),
        replace(routing_base, label="routing-sim-type-zero", sets=(*co_locale_sets, "sim_type=0")),
        replace(routing_base, label="routing-no-sim-signal", patches=(*co_operator_patch, "no-sim-signal")),
        replace(routing_base, label="routing-radio-zero", sets=(*co_locale_sets, "network_radio_type=0")),
        replace(routing_base, label="routing-radio-two", sets=(*co_locale_sets, "network_radio_type=2")),
        replace(routing_base, label="routing-radio-thirteen", sets=(*co_locale_sets, "network_radio_type=13")),
        replace(routing_base, label="routing-cellular-zero", sets=(*co_locale_sets, "cellular_strength=0")),
        replace(routing_base, label="routing-cellular-three", sets=(*co_locale_sets, "cellular_strength=3")),
        replace(routing_base, label="routing-roaming-one", sets=(*co_locale_sets, "roaming_type=1")),
        replace(routing_base, label="routing-airplane-one", sets=(*co_locale_sets, "airplane_mode_type=1")),
        replace(routing_base, label="routing-feo2-omit", omits=("feo2_query_status",)),
        replace(routing_base, label="routing-feo2-security-error", sets=(*co_locale_sets, "feo2_query_status=error_security_exception")),
        replace(routing_base, label="routing-mnc-102", sets=(*co_locale_sets, "mnc=102", "sim_mnc=102")),
        replace(routing_base, label="routing-mnc-103", sets=(*co_locale_sets, "mnc=103", "sim_mnc=103")),
        replace(routing_base, label="routing-mnc-123", sets=(*co_locale_sets, "mnc=123", "sim_mnc=123")),
        replace(routing_base, label="routing-mnc-130", sets=(*co_locale_sets, "mnc=130", "sim_mnc=130")),
    ]
    candidate_arms = [
        FactorArm("candidate", "candidate-xiaomi-301-default", device_label="xiaomi-a11", prefix="301"),
        FactorArm("candidate", "candidate-xiaomi-350-default", device_label="xiaomi-a11", prefix="350"),
        FactorArm("candidate", "candidate-random-a11-301-default", device_label="random-generic-a11", prefix="301"),
        FactorArm("candidate", "candidate-random-a11-350-default", device_label="random-generic-a11", prefix="350"),
    ]
    return [
        *combo_arms,
        *routing_arms,
        *candidate_arms,
        FactorArm("transport", "transport-requests"),
        FactorArm("transport", "transport-curl", transport="curl"),
        FactorArm("transport", "transport-curl-http1", transport="curl-http1.1"),
        FactorArm("install", "install-fresh"),
        FactorArm("install", "install-stable", stable_install=True),
        FactorArm("integrity", "integrity-signed"),
        FactorArm("integrity", "integrity-unsigned", envelope="unsigned"),
        FactorArm("integrity", "integrity-empty-h", envelope="empty-h"),
        FactorArm("integrity", "integrity-omit-wamsys", omits=wamsys_omits),
        FactorArm("integrity", "integrity-ghcr-wamsys", patches=ghcr_patches),
        FactorArm("number", "prefix-300", prefix="300"),
        FactorArm("number", "prefix-301", prefix="301"),
        FactorArm("number", "prefix-310", prefix="310"),
        FactorArm("number", "prefix-314", prefix="314"),
        FactorArm("number", "prefix-350", prefix="350"),
        FactorArm("context", "context-zero"),
        FactorArm("context", "context-co-operator", patches=("operator-co-732101",)),
        FactorArm("context", "context-co-locale", patches=("operator-co-732101",), sets=("lg=es", "lc=CO")),
        FactorArm("context", "context-no-sim-signal", patches=("no-sim-signal",)),
        FactorArm("app", "app-current"),
        FactorArm("app", "app-old-2.26.21.73", app_version="2.26.21.73"),
        FactorArm("abprop", "abprop-code-only"),
        FactorArm("abprop", "abprop-then-code", preflight="abprop"),
        FactorArm("metrics", "metrics-default"),
        FactorArm("metrics", "metrics-attempts-2", sets=('client_metrics={"attempts":2,"app_campaign_download_source":"unknown|unknown"}',)),
        FactorArm("metrics", "metrics-google-play", patches=("client-metrics-google-play",)),
        FactorArm("debug", "debug-db-one"),
        FactorArm("debug", "debug-db-zero", patches=("db-zero",)),
        FactorArm("debug", "debug-hasav-zero", sets=("hasav=0",)),
        FactorArm("debug", "debug-hasinrc-zero", sets=("hasinrc=0",)),
        FactorArm("device", "device-xiaomi-a11", device_label="xiaomi-a11"),
        FactorArm("device", "device-random-generic-a11", device_label="random-generic-a11"),
        FactorArm("device", "device-oneplus-a14", device_label="oneplus-known-a14"),
    ]


def selected_arms(groups: set[str], labels: set[str]) -> list[FactorArm]:
    arms = factor_arms()
    if groups:
        arms = [arm for arm in arms if arm.group in groups]
    if labels:
        arms = [arm for arm in arms if arm.label in labels]
    return arms


def output_paths(args: argparse.Namespace) -> tuple[Path, Path]:
    run_id = args.run_id or time.strftime("%Y%m%d-%H%M%S")
    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    return out_dir / f"{run_id}.ndjson", out_dir / f"{run_id}.summary.json"


def is_target_decision(row: dict[str, Any]) -> bool:
    return row.get("outcome") in {"sent", "no_routes"}


def sleep_after_request(args: argparse.Namespace) -> None:
    if not args.dry_run and args.sleep > 0:
        time.sleep(args.sleep + random.random() * max(args.jitter, 0))


def run_fixed_rounds(
    repo_root: Path,
    args: argparse.Namespace,
    arms: list[FactorArm],
    stable_cache: dict[str, probe.ProbeMaterial],
    handle: Any,
) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for round_index in range(1, args.samples + 1):
        round_arms = list(arms)
        random.shuffle(round_arms)
        for arm in round_arms:
            row = run_arm_once(repo_root, args, arm, stable_cache)
            row["round"] = round_index
            rows.append(row)
            line = json.dumps(row, ensure_ascii=False, sort_keys=True)
            print(line, flush=True)
            handle.write(line + "\n")
            handle.flush()
            sleep_after_request(args)
    return rows


def run_until_target_decisions(
    repo_root: Path,
    args: argparse.Namespace,
    arms: list[FactorArm],
    stable_cache: dict[str, probe.ProbeMaterial],
    handle: Any,
) -> list[dict[str, Any]]:
    max_samples = args.max_samples if args.max_samples > 0 else args.samples
    sample_counts = {arm.label: 0 for arm in arms}
    decision_counts = {arm.label: 0 for arm in arms}
    rows: list[dict[str, Any]] = []
    batch_index = 0
    while True:
        active_arms = [
            arm
            for arm in arms
            if sample_counts[arm.label] < max_samples and decision_counts[arm.label] < args.target_decisions
        ]
        if not active_arms:
            return rows
        batch_index += 1
        random.shuffle(active_arms)
        for arm in active_arms:
            row = run_arm_once(repo_root, args, arm, stable_cache)
            sample_counts[arm.label] += 1
            if is_target_decision(row):
                decision_counts[arm.label] += 1
            row["round"] = batch_index
            row["sample_index"] = sample_counts[arm.label]
            row["target_decision_index"] = decision_counts[arm.label]
            rows.append(row)
            line = json.dumps(row, ensure_ascii=False, sort_keys=True)
            print(line, flush=True)
            handle.write(line + "\n")
            handle.flush()
            sleep_after_request(args)


def main() -> int:
    parser = argparse.ArgumentParser(description="Run one-by-one SMS /v2/code factor probes with a Xiaomi Android 11 baseline.")
    parser.add_argument("--samples", type=int, default=3)
    parser.add_argument("--target-decisions", type=int, default=0, help="stop each arm after this many sent/no_routes decisions")
    parser.add_argument("--max-samples", type=int, default=0, help="per-arm cap when --target-decisions is set; defaults to --samples")
    parser.add_argument("--groups", default="", help="comma-separated factor groups")
    parser.add_argument("--labels", default="", help="comma-separated exact labels")
    parser.add_argument("--proxy", default="", help="HTTP proxy URL; WA_PROBE_PROXY_URL is used when omitted")
    parser.add_argument("--timeout", type=float, default=25)
    parser.add_argument("--sleep", type=float, default=0.6)
    parser.add_argument("--jitter", type=float, default=0.3)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--show-fields", action="store_true")
    parser.add_argument("--show-response", action="store_true")
    parser.add_argument("--out-dir", default=".temp/wa-code-param-experiments")
    parser.add_argument("--run-id", default="")
    args = parser.parse_args()

    args.proxy = probe.normalize_proxy(args.proxy or os.environ.get("WA_PROBE_PROXY_URL", ""))
    if not args.proxy and not args.dry_run:
        print(json.dumps({"error": "set WA_PROBE_PROXY_URL or pass --proxy"}, ensure_ascii=False))
        return 2
    groups = {item.strip() for item in args.groups.split(",") if item.strip()}
    labels = {item.strip() for item in args.labels.split(",") if item.strip()}
    arms = selected_arms(groups, labels)
    if not arms:
        print(json.dumps({"error": "no factor arms selected"}, ensure_ascii=False))
        return 2

    repo_root = Path(__file__).resolve().parents[1]
    stable_cache: dict[str, probe.ProbeMaterial] = {}
    ndjson_path, summary_path = output_paths(args)
    with ndjson_path.open("w", encoding="utf-8") as handle:
        if args.target_decisions > 0:
            rows = run_until_target_decisions(repo_root, args, arms, stable_cache, handle)
        else:
            rows = run_fixed_rounds(repo_root, args, arms, stable_cache, handle)
    summary = summarize(rows)
    payload = {
        "samples_per_arm": args.samples,
        "target_decisions": args.target_decisions,
        "max_samples": args.max_samples,
        "groups": sorted(groups) if groups else "all",
        "labels": [arm.label for arm in arms],
        "summary": summary,
    }
    summary_path.write_text(json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps({"result_file": str(ndjson_path), "summary_file": str(summary_path), "summary": summary}, ensure_ascii=False, sort_keys=True))
    print(markdown_table(summary))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
