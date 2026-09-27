# tech_radar plugin

tech\_radar plugin renders an admin-managed technology radar configuration file into the catalog. The YAML file is the sole source of truth: it defines the radar metadata, quadrant and ring taxonomies, and the entries with their adoption ring, movement marker, owner, and rationale.

The file is re-read on every collect, so ConfigMap updates take effect on the next sync without a redeployment. An invalid file fails the collect with errors naming the offending line and field, which should keep the previous radar snapshot in place (the catalog should never apply a failed run).

## Environment Variables

  - TECH\_RADAR\_CONFIG\_PATH (optional) - path to the radar YAML file; defaults to /etc/naira/techradar/radar.yaml.

---
Readme created from Go doc with [goreadme](https://github.com/posener/goreadme)
