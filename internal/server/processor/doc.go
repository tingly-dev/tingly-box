// Package processor hosts smart-routing op-level processors. A processor is
// a side-effect handler bound to a (Position, Operation) tuple in the
// smart_routing registry: when a rule matches and one of its ops carries a
// processor, the routing stage runs Process and lets the pipeline continue
// (implicit bypass) so the LoadBalancer forwards the mutated request.
//
// First inhabitant: VisionProxyProcessor — describes images via a
// vision-capable upstream and replaces image content blocks with text so
// downstream text-only models can serve image-bearing requests.
package processor
