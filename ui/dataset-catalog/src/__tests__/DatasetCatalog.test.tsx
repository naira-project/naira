// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and naira contributors
//
// SPDX-License-Identifier: Apache-2.0

import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import "@testing-library/jest-dom";
import DatasetCatalog from "../DatasetCatalog";
import * as api from "../api";

// Mock the api module so tests run without a real Kubernetes API server.
jest.mock("../api");
const mockedListDatasets = api.listDatasets as jest.MockedFunction<typeof api.listDatasets>;

const sampleDatasets: api.Dataset[] = [
  {
    apiVersion: "naira.io/v1alpha1",
    kind: "Dataset",
    metadata: {
      name: "prod-orders",
      namespace: "naira-system",
      creationTimestamp: "2026-01-15T10:00:00Z",
    },
    spec: {
      description: "Production orders table",
      owner: "data-team",
      sourceSystem: "openmetadata",
      sourceRegistryURL: "https://openmetadata.example.com/table/prod-orders",
      qualityScore: 92,
      tags: ["Finance.Revenue", "PII.Sensitive"],
      schema: {
        columns: [
          { name: "id", dataType: "BIGINT", description: "Primary key" },
          { name: "email", dataType: "VARCHAR", tags: ["PII.Email"] },
        ],
      },
    },
  },
  {
    apiVersion: "naira.io/v1alpha1",
    kind: "Dataset",
    metadata: {
      name: "staging-users",
      namespace: "naira-system",
      creationTimestamp: "2026-01-16T12:00:00Z",
    },
    spec: {
      description: "Staging users table",
      owner: "backend-team",
      sourceSystem: "openmetadata",
      qualityScore: 55,
      tags: ["PII.Personal"],
    },
  },
];

describe("DatasetCatalog", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("shows a loading state while fetching", () => {
    mockedListDatasets.mockReturnValue(new Promise(() => {})); // never resolves
    render(<DatasetCatalog />);
    expect(screen.getByText(/loading datasets/i)).toBeInTheDocument();
  });

  it("renders dataset cards after loading", async () => {
    mockedListDatasets.mockResolvedValue(sampleDatasets);
    render(<DatasetCatalog />);

    await waitFor(() => {
      expect(screen.getByText("prod-orders")).toBeInTheDocument();
      expect(screen.getByText("staging-users")).toBeInTheDocument();
    });
  });

  it("shows an error message when the API call fails", async () => {
    mockedListDatasets.mockRejectedValue(new Error("network error"));
    render(<DatasetCatalog />);

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("network error");
    });
  });

  it("shows a no-results message for an empty list", async () => {
    mockedListDatasets.mockResolvedValue([]);
    render(<DatasetCatalog />);

    await waitFor(() => {
      expect(screen.getByText(/no datasets found/i)).toBeInTheDocument();
    });
  });

  it("filters datasets by search query", async () => {
    mockedListDatasets.mockResolvedValue(sampleDatasets);
    render(<DatasetCatalog />);

    await waitFor(() => screen.getByText("prod-orders"));

    fireEvent.change(screen.getByRole("searchbox"), {
      target: { value: "staging" },
    });

    expect(screen.getByText("staging-users")).toBeInTheDocument();
    expect(screen.queryByText("prod-orders")).not.toBeInTheDocument();
  });

  it("filters datasets by owner", async () => {
    mockedListDatasets.mockResolvedValue(sampleDatasets);
    render(<DatasetCatalog />);

    await waitFor(() => screen.getByText("prod-orders"));

    fireEvent.change(screen.getByRole("searchbox"), {
      target: { value: "backend-team" },
    });

    expect(screen.getByText("staging-users")).toBeInTheDocument();
    expect(screen.queryByText("prod-orders")).not.toBeInTheDocument();
  });

  it("navigates to dataset detail on card click", async () => {
    mockedListDatasets.mockResolvedValue(sampleDatasets);
    render(<DatasetCatalog />);

    await waitFor(() => screen.getByText("prod-orders"));

    fireEvent.click(screen.getByLabelText("View details for prod-orders"));

    // Detail view should show schema columns
    expect(screen.getByText("id")).toBeInTheDocument();
    expect(screen.getByText("BIGINT")).toBeInTheDocument();
    expect(screen.getByText("email")).toBeInTheDocument();
  });

  it("returns to the catalog list from the detail view", async () => {
    mockedListDatasets.mockResolvedValue(sampleDatasets);
    render(<DatasetCatalog />);

    await waitFor(() => screen.getByText("prod-orders"));
    fireEvent.click(screen.getByLabelText("View details for prod-orders"));

    // Click back button
    fireEvent.click(screen.getByLabelText("Back to catalog"));

    // Should show the catalog list again
    expect(screen.getByText("prod-orders")).toBeInTheDocument();
    expect(screen.getByText("staging-users")).toBeInTheDocument();
  });

  it("displays the source link in the detail view", async () => {
    mockedListDatasets.mockResolvedValue(sampleDatasets);
    render(<DatasetCatalog />);

    await waitFor(() => screen.getByText("prod-orders"));
    fireEvent.click(screen.getByLabelText("View details for prod-orders"));

    const link = screen.getByRole("link", { name: /open prod-orders in openmetadata/i });
    expect(link).toHaveAttribute("href", "https://openmetadata.example.com/table/prod-orders");
    expect(link).toHaveAttribute("target", "_blank");
  });

  it("displays quality badge with correct styling class", async () => {
    mockedListDatasets.mockResolvedValue(sampleDatasets);
    render(<DatasetCatalog />);

    await waitFor(() => screen.getByText("prod-orders"));

    // Score 92 → high
    const badges = document.querySelectorAll(".quality-badge");
    expect(badges[0]).toHaveClass("quality-high");
    // Score 55 → medium
    expect(badges[1]).toHaveClass("quality-medium");
  });
});
