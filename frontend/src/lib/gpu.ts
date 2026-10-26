import type { Encoder, GPU } from "./api";

const driverPages: Record<string, [string, string]> = {
  nvidia: ["https://www.nvidia.com/en-us/drivers/", "Get the latest NVIDIA driver"],
  amd: ["https://www.amd.com/en/support/download/drivers.html", "Get the latest AMD driver"],
  intel: ["https://www.intel.com/content/www/us/en/support/detect.html", "Get the latest Intel driver"],
};

export function hardwareHint(encoders: Encoder[], gpus: GPU[]): { text: string; url?: string; link?: string; detail?: string; swap?: boolean } | null {
  if (encoders.length === 0 || encoders.some((e) => e.hardware && e.available && e.codec !== "av1")) return null;
  for (const g of gpus) {
    const failed = encoders.filter((e) => e.vendor === g.vendor && e.codec !== "av1" && !e.available);
    if (failed.length === 0) continue;
    const reason = failed[0].error ?? "";
    const [url, link] = driverPages[g.vendor] ?? [];
    const detail = failed
      .filter((e) => e.detail)
      .map((e) => `== ${e.name} ==\n${e.detail}`)
      .join("\n\n");
    if (reason.includes("driver too old"))
      return {
        text: `This FFmpeg was built for NVIDIA drivers that may not be out yet (${reason.replace("NVIDIA driver too old, ", "")}), and yours is ${g.driver || "older"}. An older FFmpeg works with your driver and records just as well.`,
        swap: true,
      };
    if (reason.includes("busy"))
      return { text: "The GPU encoder was busy. Close other recorders (OBS, Medal, NVIDIA instant replay) and press Test again." };
    if (reason.includes("No NVIDIA driver") || reason.includes("No AMD driver") || reason.includes("No Intel GPU"))
      return { text: `FFmpeg couldn't talk to your ${g.name}. A clean driver install usually fixes it.`, url, link };
    if (reason.includes("Not in this FFmpeg"))
      return { text: "This FFmpeg was built without GPU encoders. Use Download above to get one that has them." };
    return {
      text: `Your ${g.name} failed the test encode: ${reason || "no reason given"}. Press Test again once. If it still fails, copy the details and send them over.`,
      detail: detail || undefined,
    };
  }
  return null;
}
