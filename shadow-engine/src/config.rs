use serde::Deserialize;
use std::path::Path;

#[derive(Deserialize, Debug)]
pub struct ArbiterConfig {
    pub version: String,
    pub engine: EngineConfig,
    pub sandbox_isolation: SandboxIsolationConfig,
    pub telemetry_and_drifting: TelemetryConfig,
    pub cryptographic_ledger: LedgerConfig,
}

#[derive(Deserialize, Debug)]
pub struct EngineConfig {
    pub mode: String,
    pub ipc_transport: String,
    pub socket_path: String,
    pub max_parallel_sandboxes: u32,
}

#[derive(Deserialize, Debug)]
pub struct SandboxIsolationConfig {
    pub allow_external_networking: bool,
    pub virtual_clock_speedup_factor: f64,
    pub drift_tolerance_ms: u64,
    pub state_fork_mode: String,
}

#[derive(Deserialize, Debug)]
pub struct DivergenceMetric {
    pub metric: String,
    pub threshold: f64,
}

#[derive(Deserialize, Debug)]
pub struct KillSwitchConditions {
    pub max_shadow_drawdown_percent: f64,
    pub consecutive_hallucination_blocks: u32,
    pub unmapped_ticker_executions: bool,
}

#[derive(Deserialize, Debug)]
pub struct TelemetryConfig {
    pub divergence_metrics: Vec<DivergenceMetric>,
    pub kill_switch_conditions: KillSwitchConditions,
}

#[derive(Deserialize, Debug)]
pub struct LedgerConfig {
    pub append_only_logging: bool,
    pub hashing_algorithm: String,
    pub merkle_tree_flush_interval_ms: u64,
    pub storage_backend: String,
    pub export_format: String,
}

impl ArbiterConfig {
    pub fn load(path: &str) -> Result<Self, Box<dyn std::error::Error>> {
        let contents = std::fs::read_to_string(Path::new(path))?;
        let config: ArbiterConfig = serde_yaml::from_str(&contents)?;
        Ok(config)
    }

    pub fn slippage_threshold(&self) -> f64 {
        self.telemetry_and_drifting
            .divergence_metrics
            .iter()
            .find(|m| m.metric == "slippage_delta")
            .map(|m| m.threshold)
            .unwrap_or(0.0015)
    }

    pub fn max_drawdown_percent(&self) -> f64 {
        self.telemetry_and_drifting
            .kill_switch_conditions
            .max_shadow_drawdown_percent
    }
}
