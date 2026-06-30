interface TradePayload {
  agent_id: string;
  timestamp: number;
  asset_class: string;
  ticker: string;
  order_type: "MARKET" | "LIMIT" | "STOP" | "STOP_LIMIT";
  quantity: number;
  price: number;
  context_window_reasoning: string;
  crypto_checksum: string;
}

interface RiskVerdict {
  decision: "ALLOW" | "KILL";
  reason: string;
  latency_ms: number;
}

interface TelemetryHealth {
  total_signals_read: number;
  successful_drops: number;
  processing_failures: number;
}

export class ArbiterClient {
  private baseUrl: string;
  private timeoutMs: number;

  constructor(baseUrl: string, timeoutMs: number = 5) {
    this.baseUrl = baseUrl.replace(/\/$/, "");
    this.timeoutMs = timeoutMs;
  }

  async checkRisk(params: {
    agentId: string;
    ticker: string;
    orderType: TradePayload["order_type"];
    quantity: number;
    price: number;
    reasoning: string;
    assetClass?: string;
    checksum?: string;
  }): Promise<RiskVerdict> {
    const payload: TradePayload = {
      agent_id: params.agentId,
      timestamp: Date.now() * 1_000_000,
      asset_class: params.assetClass ?? "us_equity",
      ticker: params.ticker,
      order_type: params.orderType,
      quantity: params.quantity,
      price: params.price,
      context_window_reasoning: params.reasoning,
      crypto_checksum: params.checksum ?? "",
    };

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), this.timeoutMs);

    try {
      const resp = await fetch(`${this.baseUrl}/v1/risk/check`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
        signal: controller.signal,
      });
      return (await resp.json()) as RiskVerdict;
    } finally {
      clearTimeout(timeout);
    }
  }

  async exportComplianceLedger(authToken: string): Promise<string> {
    const resp = await fetch(`${this.baseUrl}/v1/compliance/export`, {
      headers: { "X-Arbiter-Auth": authToken },
    });
    if (!resp.ok) throw new Error(`Export failed: ${resp.status}`);
    return resp.text();
  }

  async getTelemetry(): Promise<TelemetryHealth> {
    const resp = await fetch(`${this.baseUrl}/v1/telemetry/health`);
    return (await resp.json()) as TelemetryHealth;
  }
}
