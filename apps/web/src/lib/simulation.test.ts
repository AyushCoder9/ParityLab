import assert from "node:assert/strict";
import test from "node:test";
import { getRunState, getVisibleEvents, scenarios } from "./simulation.ts";

test("seeded scenarios are deterministic and ordered", () => {
  for (const scenario of scenarios) {
    const times = scenario.events.map((event) => event.at);
    assert.deepEqual(times, [...times].sort((a, b) => a - b));
    assert.equal(getVisibleEvents(scenario, 0).length, 1);
    assert.equal(getVisibleEvents(scenario, 1).length, scenario.events.length);
  }
});

test("run state exposes divergence and eventual convergence", () => {
  const scenario = scenarios[0];
  assert.equal(getRunState(scenario, 0.1), "running");
  assert.equal(getRunState(scenario, 0.55), "diverged");
  assert.equal(getRunState(scenario, 1), "verified");
});

test("timeline clamps progress safely", () => {
  const scenario = scenarios[0];
  assert.deepEqual(getVisibleEvents(scenario, -5), getVisibleEvents(scenario, 0));
  assert.deepEqual(getVisibleEvents(scenario, 5), scenario.events);
});
