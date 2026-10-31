-- Development seed: one agency, one zone (a rough box around Mount Arafat).
INSERT INTO tenants (id, name, slug)
VALUES ('11111111-1111-1111-1111-111111111111', 'Demo Agency', 'demo-agency')
ON CONFLICT DO NOTHING;

INSERT INTO zones (id, tenant_id, name, coordinates_polygon)
VALUES (
    '22222222-2222-2222-2222-222222222222',
    '11111111-1111-1111-1111-111111111111',
    'Sector A',
    '[{"lat":21.3560,"lng":39.9810},{"lat":21.3560,"lng":39.9860},{"lat":21.3520,"lng":39.9860},{"lat":21.3520,"lng":39.9810}]'
)
ON CONFLICT DO NOTHING;
