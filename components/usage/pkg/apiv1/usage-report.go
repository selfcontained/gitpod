// Copyright (c) 2022 Gitpod GmbH. All rights reserved.
// Licensed under the GNU Affero General Public License (AGPL).
// See License-AGPL.txt in the project root for license information.

package apiv1

import (
	"context"
	"fmt"
	"github.com/gitpod-io/gitpod/common-go/log"
	"github.com/gitpod-io/gitpod/usage/pkg/db"
	"gorm.io/gorm"
	"time"
)

type InvalidSession struct {
	Reason  string
	Session db.WorkspaceInstanceForUsage
}

type UsageReport struct {
	GenerationTime time.Time

	From time.Time
	To   time.Time

	RawSessions     []db.WorkspaceInstanceForUsage
	InvalidSessions []InvalidSession

	UsageRecords []db.WorkspaceInstanceUsage
}

func NewReportGenerator(conn *gorm.DB, pricer *WorkspacePricer) *ReportGenerator {
	return &ReportGenerator{
		conn:    conn,
		pricer:  pricer,
		nowFunc: time.Now,
	}
}

type ReportGenerator struct {
	conn    *gorm.DB
	pricer  *WorkspacePricer
	nowFunc func() time.Time
}

func (g *ReportGenerator) GenerateUsageReport(ctx context.Context, from, to time.Time) (UsageReport, error) {
	now := g.nowFunc().UTC()
	log.Infof("Gathering usage data from %s to %s", from, to)

	report := UsageReport{
		GenerationTime: now,
		From:           from,
		To:             to,
	}

	instances, err := db.ListWorkspaceInstancesInRange(ctx, g.conn, from, to)
	if err != nil {
		return report, fmt.Errorf("failed to list instances from db: %w", err)
	}
	report.RawSessions = instances

	valid, invalid := validateInstances(instances)
	report.InvalidSessions = invalid

	if len(invalid) > 0 {
		log.WithField("invalid_workspace_instances", invalid).Errorf("Detected %d invalid instances. These will be skipped in the current run.", len(invalid))
	}
	log.WithField("workspace_instances", instances).Debug("Successfully loaded workspace instances.")

	trimmed := trimStartStopTime(valid, from, to)

	report.UsageRecords = instancesToUsageRecords(trimmed, g.pricer, now)
	return report, nil
}

func validateInstances(instances []db.WorkspaceInstanceForUsage) (valid []db.WorkspaceInstanceForUsage, invalid []InvalidSession) {
	for _, i := range instances {
		// i is a pointer to the current element, we need to assign it to ensure we're copying the value, not the current pointer.
		instance := i

		// Each instance must have a start time, without it, we do not have a baseline for usage computation.
		if !instance.CreationTime.IsSet() {
			invalid = append(invalid, InvalidSession{
				Reason:  "missing creation time",
				Session: instance,
			})
			continue
		}

		start := instance.CreationTime.Time()

		// Currently running instances do not have a stopped time set, so we ignore these.
		if instance.StoppedTime.IsSet() {
			stop := instance.StoppedTime.Time()
			if stop.Before(start) {
				invalid = append(invalid, InvalidSession{
					Reason:  "stop time is before start time",
					Session: instance,
				})
				continue
			}
		}

		valid = append(valid, instance)
	}
	return valid, invalid
}

// trimStartStopTime ensures that start time or stop time of an instance is never outside of specified start or stop time range.
func trimStartStopTime(instances []db.WorkspaceInstanceForUsage, maximumStart, minimumStop time.Time) []db.WorkspaceInstanceForUsage {
	var updated []db.WorkspaceInstanceForUsage

	for _, instance := range instances {
		if instance.CreationTime.Time().Before(maximumStart) {
			instance.CreationTime = db.NewVarcharTime(maximumStart)
		}

		if instance.StoppedTime.Time().After(minimumStop) {
			instance.StoppedTime = db.NewVarcharTime(minimumStop)
		}

		updated = append(updated, instance)
	}
	return updated
}
