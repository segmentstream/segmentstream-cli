<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/assets/segmentstream-on-dark.svg">
    <img alt="SegmentStream" src=".github/assets/segmentstream-on-light.svg" width="320">
  </picture>
</p>

<p align="center">
  The composable, transparent, AI-ready marketing measurement stack —<br>
  running in your own data warehouse.
</p>

<p align="center">
  <a href="https://segmentstream.com">Website</a> ·
  <a href="https://segmentstream.com/pricing">Pricing</a> ·
  <a href="https://segmentstream.com/about">About</a> ·
  <a href="#license">License</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/license-BSL%201.1-6366F1" alt="License: BSL 1.1">
  <img src="https://img.shields.io/badge/warehouse--native-000000" alt="Warehouse-native">
  <img src="https://img.shields.io/badge/AI--ready-MCP-6366F1" alt="AI-ready · MCP">
  <img src="https://img.shields.io/badge/built%20by-SegmentStream-18181B" alt="Built by SegmentStream">
</p>

---

The full Measurement Engine on your terminal. Every model is readable SQL. Every
number is auditable. Your data never leaves your infrastructure.

```sh
curl -fsSL https://segmentstream.com/cli/install.sh | sh
```

Prefer to read it first? Inspect the script at
[segmentstream.com/cli/install.sh](https://segmentstream.com/cli/install.sh)
(mirrors `install.sh` in this repo).

## Why

> Marketing measurement drifted into a belief system — teams trusting
> attribution models and MMM frameworks the way people trust horoscopes.
> Confident. Detailed. Mostly fiction.

SegmentStream is the opposite of a black box. It runs inside **your** warehouse,
on **your** data, with **every** model published as inspectable SQL. We work for
the advertiser — that is the only side we work for. A measurement number you
cannot defend is a measurement number you cannot use.

## What makes it different

- **Transparent** — No black boxes. Every attribution and measurement model is
  readable SQL; every figure traces back to deterministic data you own. Modeled
  numbers are labeled as modeled.
- **Composable** — Warehouse-native. Sources are dbt packages that map your raw
  tables to a typed canonical contract — mix built-in and custom freely.
- **Extendable** — Add a source by writing one SQL model against the contract;
  contract tests verify it. Bring your own identity keys, dimensions and
  attribution models.
- **AI-ready** — A CLI with text-based, version-controlled config and SQL models
  — agent-operable end to end. Source scaffolds are written for humans *and*
  agents. Works with any MCP client.

## How it works

1. **Initialize** — scaffold a version-controlled project.
2. **Connect** — authenticate to your warehouse (BigQuery today; Snowflake &
   Databricks on the roadmap); credentials stay in your OS keychain.
3. **Add sources** — events, costs and conversions from built-in or custom
   connectors.
4. **Configure** — identity keys, dimensions and attribution models, built-in or
   custom SQL.
5. **Run** — identity stitching, attribution and reporting execute locally, in
   your warehouse.
6. **Activate** — send conversions back to ad platforms via server-side
   Conversions APIs.

## Status & roadmap

Legend: ✅ Live · 🚧 Building · 📋 Planned — directional, not dated. Detail lives
on the [public roadmap board](#).

### Warehouses

| Adapter           | Status      |
| ----------------- | ----------- |
| Google BigQuery   | ✅ Live      |
| Snowflake         | 📋 Planned  |
| Databricks        | 📋 Planned  |

### Sources (contract-based, warehouse-native)

| Capability         | Status      | Notes                                          |
| ------------------ | ----------- | ---------------------------------------------- |
| Event sources      | ✅ Live      | Typed contract + dbt scaffold, contract-verified |
| Cost sources       | 📋 Planned  | Same contract rails                            |
| Conversion sources | 📋 Planned  | Simple / custom / combined / lead-scoring      |

### Identity graph (deterministic — no fingerprinting)

| Capability            | Status      | Notes                                          |
| --------------------- | ----------- | ---------------------------------------------- |
| Default identity keys | 🚧 Building | Built-in keys for cross-device stitching       |
| Custom identity keys  | 🚧 Building | Bring-your-own keys with configurable confidence windows |

### Attribution, dimensions & activation

| Capability                         | Status     | Notes                                   |
| ---------------------------------- | ---------- | --------------------------------------- |
| Attribution models (SQL)           | 📋 Planned | Built-in and custom SQL models          |
| Custom & grouped dimensions        | 📋 Planned | For reports and optimization portfolios |
| Conversion export / Conversions API| 📋 Planned | Server-side Conversions API destinations|
| Incrementality / budget allocation | 📋 Planned | Geo holdouts, marginal ROAS, automated allocation |

## License

SegmentStream is **source-available** under the
[Business Source License 1.1](LICENSE) — not an OSI open-source license, and we
don't call it one. One license over the whole repository: no second-class
folders, no "look but don't touch" code.

- ✅ Read all the source. Self-host it. Use it in production for your own
  marketing — commercial use included.
- ✅ Modify and extend it for your own use.
- 🚫 You may not offer it to third parties as a competing hosted or managed
  service.
- ⏳ Four years after each version is published, that version converts to the
  Apache License, Version 2.0.

## ☁️ SegmentStream Cloud & Commercial

This project is built and maintained by the team behind
**[SegmentStream](https://segmentstream.com)** — the independent measurement
platform for advertisers, in production since 2018.

Want a fully-managed deployment, enterprise support and SLAs, hands-on
onboarding, or to do something the source-available license doesn't permit? We
offer SegmentStream Cloud and commercial licensing.

**[See plans & talk to us →](https://segmentstream.com/pricing)**

---

Built by [SegmentStream](https://segmentstream.com/about) — the independent
measurement platform for advertisers. Learn the discipline first:
[segmentstream.com](https://segmentstream.com).
