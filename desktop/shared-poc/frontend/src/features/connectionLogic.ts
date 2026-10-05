import {
  Availability,
  OAuthPasswordState,
  PortState,
  type ConnectionSnapshot,
  type OperationCapability,
  type PortObservation,
} from '../api/desktopApi'

export function connectionCapability(
  snapshot: ConnectionSnapshot | null,
  name: string,
): OperationCapability | undefined {
  return snapshot?.operations?.find((operation) => operation.name === name)
}

export function canPerformConnection(name: string, snapshot: ConnectionSnapshot | null): boolean {
  const capability = connectionCapability(snapshot, name)
  return capability?.availability === Availability.AvailabilityAvailable
}

export function safeMCPURL(raw: string | undefined, publicOnly = false): string {
  if (!raw) return ''
  try {
    const url = new URL(raw)
    if (url.username || url.password || url.search || url.hash || url.pathname !== '/mcp') return ''
    if (publicOnly) return url.protocol === 'https:' ? url.toString() : ''
    const loopback = url.hostname === '127.0.0.1' || url.hostname === '::1' || url.hostname === '[::1]' || url.hostname === 'localhost'
    return url.protocol === 'http:' && loopback ? url.toString() : ''
  } catch {
    return ''
  }
}

export function namedOriginFromPublicMCPURL(raw: string | undefined): string {
  const safe = safeMCPURL(raw, true)
  if (!safe) return ''
  return new URL(safe).origin
}

export function validNamedOrigin(raw: string): boolean {
  try {
    const value = raw.trim()
    const url = new URL(value)
    return (
      value === url.origin &&
      url.protocol === 'https:' &&
      !url.username &&
      !url.password &&
      !url.search &&
      !url.hash
    )
  } catch {
    return false
  }
}

export function actionablePortObservation(
  observation: PortObservation | null,
  candidate: number,
  currentPort: number,
): boolean {
  if (!Number.isInteger(candidate) || candidate < 1 || candidate > 65535) return false
  if (!observation || observation.observedPort !== candidate) return false
  if (candidate === currentPort) return observation.state === PortState.PortOwnedByNext || observation.state === PortState.PortAvailable
  return observation.state === PortState.PortAvailable
}

export function connectorReadiness(snapshot: ConnectionSnapshot | null, publicEndpointState: string): {
  local: boolean
  public: boolean
} {
  if (!snapshot) return { local: false, public: false }
  const local =
    snapshot.coreRunning === true &&
    snapshot.coreHealth === 'healthy' &&
    snapshot.portObservation.state === PortState.PortOwnedByNext &&
    safeMCPURL(snapshot.localMCPURL) !== ''
  const publicReady =
    local &&
    safeMCPURL(snapshot.publicMCPURL, true) !== '' &&
    snapshot.oauthPasswordState === OAuthPasswordState.OAuthPasswordStored &&
    publicEndpointState === 'reachable'
  return { local, public: publicReady }
}
