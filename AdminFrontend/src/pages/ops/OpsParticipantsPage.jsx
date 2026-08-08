import CrudResource from "../../components/CrudResource";
import { Badge } from "../../components/ui";
import { opsApi } from "../../api/endpoints";

// OpsParticipantsPage — every participant across all families. Ops may edit or
// delete any without restriction (per the requirement).
export default function OpsParticipantsPage() {
  return (
    <CrudResource
      title="Participants"
      subtitle="All people across families. Edit or remove any — Ops has no restriction."
      api={opsApi.participants}
      canCreate={false}
      columns={[
        { key: "name", label: "Name" },
        { key: "familyPhone", label: "Family" },
        { key: "gender", label: "Gender" },
        { key: "city", label: "City" },
        { key: "aadhaarMasked", label: "Aadhaar" },
        { key: "deleted", label: "State", render: (r) => (r.deleted ? <Badge tone="err">removed</Badge> : <Badge tone="open">active</Badge>) },
      ]}
      fields={[
        { key: "name", label: "Name" },
        {
          key: "gender",
          label: "Gender",
          type: "select",
          options: ["female", "male", "other"].map((v) => ({ value: v, label: v })),
        },
        { key: "relationship", label: "Relationship" },
        { key: "school", label: "School" },
        { key: "city", label: "City" },
        { key: "guru", label: "Guru" },
      ]}
    />
  );
}
