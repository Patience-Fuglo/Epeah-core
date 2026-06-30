use tokio_vsock::VsockListener;
use tokio::io::AsyncWriteExt;
use tokio_util::codec::{FramedRead, LinesCodec};
use futures::StreamExt;
use std::sync::Arc;
use std::collections::HashMap;

use crate::config::ArbiterConfig;
use crate::divergence::DivergenceEngine;
use crate::guardrail::ContextGuardrail;
use crate::ledger::AuditLedger;
use crate::payload::InboundTradePayload;
use crate::sandbox::ShadowSandbox;
use crate::signal::KillSignalPayload;

pub struct EnclaveVsockListener {
    port: u32,
    slippage_threshold: f64,
    max_drawdown: f64,
}

impl EnclaveVsockListener {
    pub fn new(port: u32, slippage_threshold: f64, max_drawdown: f64) -> Self {
        Self {
            port,
            slippage_threshold,
            max_drawdown,
        }
    }

    pub async fn run(&self) -> Result<(), Box<dyn std::error::Error>> {
        let listener = VsockListener::bind(0xFFFFFFFF, self.port)?;
        println!("Arbiter Enclave Active: Listening on secure vsock port {}", self.port);

        let slippage = self.slippage_threshold;
        let drawdown = self.max_drawdown;

        while let Ok((stream, addr)) = listener.accept().await {
            println!("Secure connection established with host CID: {}", addr.cid());

            tokio::spawn(async move {
                let (read_half, mut write_half) = tokio::io::split(stream);
                let mut framed = FramedRead::new(read_half, LinesCodec::new());
                let mut ledger = AuditLedger::new("/var/log/arbiter/compliance_ledger.json");

                while let Some(Ok(line)) = framed.next().await {
                    if let Ok(payload) = serde_json::from_str::<InboundTradePayload>(&line) {
                        if ContextGuardrail::is_hallucinating_or_compromised(&payload.context_window_reasoning) {
                            let _ = ledger.commit_entry(&payload.agent_id, &payload.ticker, "REJECTED_SEMANTIC", &line);

                            let signal = serde_json::json!({
                                "signal_type": "KILL_FLATTEN",
                                "agent_id": payload.agent_id,
                                "ticker": payload.ticker,
                                "violation_reason": "Semantic Guardrail Violation: Hallucination or Injection Loop Detected",
                                "timestamp": chrono::Utc::now().timestamp_nanos_opt().unwrap_or(0)
                            });
                            if let Ok(mut packet) = serde_json::to_vec(&signal) {
                                packet.push(b'\n');
                                let _ = write_half.write_all(&packet).await;
                                let _ = write_half.flush().await;
                            }
                            continue;
                        }

                        let engine = DivergenceEngine::new(slippage, drawdown);

                        let sandbox = ShadowSandbox {
                            sandbox_id: 1,
                            base_state: Arc::new(HashMap::new()),
                            local_mutations: HashMap::new(),
                        };

                        match engine.evaluate_deviation(&payload, &sandbox) {
                            Ok(true) => {
                                let _ = ledger.commit_entry(&payload.agent_id, &payload.ticker, "REJECTED_DIVERGENCE", &line);

                                let signal = serde_json::json!({
                                    "signal_type": "KILL_FLATTEN",
                                    "agent_id": payload.agent_id,
                                    "ticker": payload.ticker,
                                    "violation_reason": format!("Slippage threshold exceeded (Delta > {:.1}bps)", slippage * 10000.0),
                                    "timestamp": chrono::Utc::now().timestamp_nanos_opt().unwrap_or(0)
                                });
                                if let Ok(mut packet) = serde_json::to_vec(&signal) {
                                    packet.push(b'\n');
                                    let _ = write_half.write_all(&packet).await;
                                    let _ = write_half.flush().await;
                                }
                            }
                            Err(e) => {
                                eprintln!("Divergence evaluation error: {}", e);
                            }
                            Ok(false) => {
                                let _ = ledger.commit_entry(&payload.agent_id, &payload.ticker, "APPROVED", &line);
                            }
                        }
                    }
                }
            });
        }
        Ok(())
    }
}
