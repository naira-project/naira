export const STAT_CARDS = [
  { label: 'Active Models', value: '24', delta: '+3 this week', color: 'primary.main' },
  { label: 'Experiments', value: '138', delta: '+12 this week', color: 'secondary.main' },
  { label: 'Deployments', value: '9', delta: '2 pending', color: 'success.main' },
  { label: 'Alerts', value: '3', delta: '1 critical', color: 'warning.main' },
];

export const WORK_ITEMS = [
  { id: 'WI-001', title: 'Retrain sentiment model on Q1 data', status: 'In Progress', priority: 'High' },
  { id: 'WI-002', title: 'Review drift report for recommendation engine', status: 'Pending Review', priority: 'Medium'},
  { id: 'WI-003', title: 'Update feature store schema for churn model', status: 'Done', priority: 'Low'},
  { id: 'WI-004', title: 'Benchmark new embedding model v2', status: 'In Progress', priority: 'High' },
  { id: 'WI-005', title: 'Document experiment tracking guidelines', status: 'Pending Review', priority: 'Medium'},
  { id: 'WI-006', title: 'Archive deprecated pipeline artifacts', status: 'Done', priority: 'Low'},
  { id: 'WI-007', title: 'Set up A/B test for pricing model rollout', status: 'In Progress', priority: 'High' },
];



export const NAV_CATALOG = ['Datasets', 'RAGs', 'Prompts', 'Artifacts', 'Model serving'];

export const MODEL_TYPE = ["Multi-modal", "Text-to-Image", "Text-to-Video", "Audio-to-text"];

export const MODEL_KIND = ["Raw OSS", "Fine-Tuned OSS", "External GP"];

export const RISK = ["High", "Medium", "Low", "Unspecified"];

export const DOMAIN = ["Sales", "HR", "Procurement", "IT", "Marketing"];

export const CATEGORIES = ["Model Name", "Params", "Type", "Domain", "Version", "Kind", "Risk", "Details"];
