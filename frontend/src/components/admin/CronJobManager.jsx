import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Modal } from "@/components/common";
import { buildApiBase } from "@/lib/api";
import { ArrowLeft, Plus, Play, Edit2, Trash2, Clock, RefreshCw, Check, X } from "lucide-react";

export function CronJobManager({ node, privilegedToken, onBack }) {
  const [jobs, setJobs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [showModal, setShowModal] = useState(false);
  const [editingJob, setEditingJob] = useState(null);
  const [formData, setFormData] = useState({
    name: "",
    command: "",
    enabled: true,
    schedule: {
      second: "0",
      minute: "*",
      hour: "*",
      day: "*",
      month: "*",
      weekday: "*",
    },
  });
  const [saving, setSaving] = useState(false);

  const loadJobs = async () => {
    setLoading(true);
    setError("");
    try {
      const response = await fetch(
        `${buildApiBase()}/cluster-manager/nodes/${encodeURIComponent(node.name)}/cron`,
        {
          credentials: "include",
          headers: {
            "X-Privileged-Token": privilegedToken,
          },
        }
      );

      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || "Failed to load cron jobs");
      }

      const data = await response.json();
      setJobs(data || []);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadJobs();
  }, [node.name, privilegedToken]);

  const resetForm = () => {
    setFormData({
      name: "",
      command: "",
      enabled: true,
      schedule: {
        second: "0",
        minute: "*",
        hour: "*",
        day: "*",
        month: "*",
        weekday: "*",
      },
    });
    setEditingJob(null);
  };

  const handleCreate = () => {
    resetForm();
    setShowModal(true);
  };

  const handleEdit = (job) => {
    setEditingJob(job);
    setFormData({
      name: job.name,
      command: job.command,
      enabled: job.enabled,
      schedule: { ...job.schedule },
    });
    setShowModal(true);
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!formData.name || !formData.command) return;

    setSaving(true);
    setError("");

    try {
      const url = editingJob
        ? `${buildApiBase()}/cluster-manager/nodes/${encodeURIComponent(node.name)}/cron/${editingJob.id}`
        : `${buildApiBase()}/cluster-manager/nodes/${encodeURIComponent(node.name)}/cron`;

      const response = await fetch(url, {
        method: editingJob ? "PUT" : "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Privileged-Token": privilegedToken,
        },
        credentials: "include",
        body: JSON.stringify(formData),
      });

      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || "Failed to save cron job");
      }

      setShowModal(false);
      resetForm();
      await loadJobs();
    } catch (err) {
      setError(err.message);
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (job) => {
    if (!confirm(`Delete cron job "${job.name}"?`)) return;

    try {
      const response = await fetch(
        `${buildApiBase()}/cluster-manager/nodes/${encodeURIComponent(node.name)}/cron/${job.id}`,
        {
          method: "DELETE",
          headers: {
            "X-Privileged-Token": privilegedToken,
          },
          credentials: "include",
        }
      );

      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || "Failed to delete cron job");
      }

      await loadJobs();
    } catch (err) {
      setError(err.message);
    }
  };

  const handleRun = async (job) => {
    try {
      const response = await fetch(
        `${buildApiBase()}/cluster-manager/nodes/${encodeURIComponent(node.name)}/cron/${job.id}/run`,
        {
          method: "POST",
          headers: {
            "X-Privileged-Token": privilegedToken,
          },
          credentials: "include",
        }
      );

      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || "Failed to run cron job");
      }

      // Reload to see updated status
      setTimeout(loadJobs, 1000);
    } catch (err) {
      setError(err.message);
    }
  };

  const formatSchedule = (schedule) => {
    if (!schedule) return "—";
    return `${schedule.minute} ${schedule.hour} ${schedule.day} ${schedule.month} ${schedule.weekday}`;
  };

  const formatLastRun = (lastRun) => {
    if (!lastRun) return "Never";
    return new Date(lastRun).toLocaleString();
  };

  return (
    <div>
      <div className="flex items-center gap-4 mb-4">
        <Button variant="outline" size="sm" onClick={onBack}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          Back
        </Button>
        <h2 className="text-xl font-semibold flex items-center gap-2">
          <Clock className="w-5 h-5" />
          Cron Jobs: {node?.name}
        </h2>
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle>Scheduled Jobs</CardTitle>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={loadJobs} disabled={loading}>
              <RefreshCw className={`w-4 h-4 mr-2 ${loading ? "animate-spin" : ""}`} />
              Refresh
            </Button>
            <Button size="sm" onClick={handleCreate}>
              <Plus className="w-4 h-4 mr-2" />
              Add Job
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {error && <p className="text-destructive text-sm mb-4">{error}</p>}

          {loading ? (
            <p className="text-muted-foreground py-4">Loading jobs...</p>
          ) : jobs.length === 0 ? (
            <p className="text-muted-foreground py-4">No cron jobs configured</p>
          ) : (
            <div className="space-y-2">
              {/* Header - hidden on mobile */}
              <div className="hidden lg:grid lg:grid-cols-[2fr_2fr_1fr_1.5fr_1fr_150px] gap-4 px-4 py-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                <div>Name</div>
                <div>Schedule</div>
                <div className="text-center">Status</div>
                <div>Last Run</div>
                <div className="text-center">Enabled</div>
                <div className="text-center">Actions</div>
              </div>
              {jobs.map((job) => (
                <div
                  key={job.id}
                  className="flex flex-col lg:grid lg:grid-cols-[2fr_2fr_1fr_1.5fr_1fr_150px] gap-2 lg:gap-4 px-4 py-3 bg-secondary rounded-md lg:items-center"
                >
                  <div className="flex justify-between lg:block">
                    <span className="text-xs text-muted-foreground lg:hidden">Name:</span>
                    <span className="font-medium truncate">{job.name}</span>
                  </div>
                  <div className="flex justify-between lg:block">
                    <span className="text-xs text-muted-foreground lg:hidden">Schedule:</span>
                    <code className="text-xs bg-background px-2 py-1 rounded font-mono">
                      {formatSchedule(job.schedule)}
                    </code>
                  </div>
                  <div className="flex justify-between lg:justify-center items-center">
                    <span className="text-xs text-muted-foreground lg:hidden">Status:</span>
                    {job.last_status ? (
                      job.last_status === "success" ? (
                        <span className="flex items-center gap-1 text-green-400 text-sm">
                          <Check className="w-4 h-4" /> OK
                        </span>
                      ) : (
                        <span className="flex items-center gap-1 text-red-400 text-sm">
                          <X className="w-4 h-4" /> Fail
                        </span>
                      )
                    ) : (
                      <span className="text-muted-foreground text-sm">—</span>
                    )}
                  </div>
                  <div className="flex justify-between lg:block">
                    <span className="text-xs text-muted-foreground lg:hidden">Last Run:</span>
                    <span className="text-sm text-muted-foreground">
                      {formatLastRun(job.last_run)}
                    </span>
                  </div>
                  <div className="flex justify-between lg:justify-center items-center">
                    <span className="text-xs text-muted-foreground lg:hidden">Enabled:</span>
                    <span className={job.enabled ? "text-green-400" : "text-muted-foreground"}>
                      {job.enabled ? "Yes" : "No"}
                    </span>
                  </div>
                  <div className="flex gap-1 justify-center mt-2 lg:mt-0">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleRun(job)}
                      title="Run now"
                    >
                      <Play className="w-4 h-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleEdit(job)}
                      title="Edit"
                    >
                      <Edit2 className="w-4 h-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-destructive hover:text-destructive"
                      onClick={() => handleDelete(job)}
                      title="Delete"
                    >
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Create/Edit Modal */}
      <Modal
        open={showModal}
        onClose={() => {
          setShowModal(false);
          resetForm();
        }}
        title={editingJob ? "Edit Cron Job" : "Create Cron Job"}
        description="Configure the schedule and command for this job."
        className="max-w-lg"
      >
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              placeholder="e.g. Daily Backup"
              value={formData.name}
              onChange={(e) => setFormData((p) => ({ ...p, name: e.target.value }))}
              autoFocus
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="command">Command</Label>
            <Input
              id="command"
              placeholder="e.g. /usr/local/bin/backup.sh"
              value={formData.command}
              onChange={(e) => setFormData((p) => ({ ...p, command: e.target.value }))}
              className="font-mono"
            />
          </div>

          <div className="space-y-2">
            <Label>Schedule (Cron Format)</Label>
            <div className="grid grid-cols-5 gap-2">
              <div>
                <Label className="text-xs text-muted-foreground">Minute</Label>
                <Input
                  placeholder="*"
                  value={formData.schedule.minute}
                  onChange={(e) =>
                    setFormData((p) => ({
                      ...p,
                      schedule: { ...p.schedule, minute: e.target.value },
                    }))
                  }
                  className="font-mono text-center"
                />
              </div>
              <div>
                <Label className="text-xs text-muted-foreground">Hour</Label>
                <Input
                  placeholder="*"
                  value={formData.schedule.hour}
                  onChange={(e) =>
                    setFormData((p) => ({
                      ...p,
                      schedule: { ...p.schedule, hour: e.target.value },
                    }))
                  }
                  className="font-mono text-center"
                />
              </div>
              <div>
                <Label className="text-xs text-muted-foreground">Day</Label>
                <Input
                  placeholder="*"
                  value={formData.schedule.day}
                  onChange={(e) =>
                    setFormData((p) => ({
                      ...p,
                      schedule: { ...p.schedule, day: e.target.value },
                    }))
                  }
                  className="font-mono text-center"
                />
              </div>
              <div>
                <Label className="text-xs text-muted-foreground">Month</Label>
                <Input
                  placeholder="*"
                  value={formData.schedule.month}
                  onChange={(e) =>
                    setFormData((p) => ({
                      ...p,
                      schedule: { ...p.schedule, month: e.target.value },
                    }))
                  }
                  className="font-mono text-center"
                />
              </div>
              <div>
                <Label className="text-xs text-muted-foreground">Weekday</Label>
                <Input
                  placeholder="*"
                  value={formData.schedule.weekday}
                  onChange={(e) =>
                    setFormData((p) => ({
                      ...p,
                      schedule: { ...p.schedule, weekday: e.target.value },
                    }))
                  }
                  className="font-mono text-center"
                />
              </div>
            </div>
            <p className="text-xs text-muted-foreground">
              Standard cron format: minute hour day month weekday
            </p>
          </div>

          <div className="flex items-center gap-2">
            <Switch
              id="enabled"
              checked={formData.enabled}
              onCheckedChange={(checked) => setFormData((p) => ({ ...p, enabled: checked }))}
            />
            <Label htmlFor="enabled">Enabled</Label>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}

          <div className="flex justify-end gap-2 pt-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setShowModal(false);
                resetForm();
              }}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={saving || !formData.name || !formData.command}
            >
              {saving ? "Saving..." : editingJob ? "Update" : "Create"}
            </Button>
          </div>
        </form>
      </Modal>
    </div>
  );
}
