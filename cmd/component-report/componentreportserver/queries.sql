insert
into
    deads_feature (component_id,
                   feature_name)
select
    id,
    case
        when id = id then 'unassigned'
        end
from
    deads_components;


insert
into
    deads_feature (component_id, feature_name)
values
    (1, 'operator'),
    (1, 'alerts'),
    (1, 'disruption'),
    (1, 'apply'),
    (1, 'crds'),
    (4, 'operator'),
    (4, 'alerts'),
    (4, 'disruption'),
    (5, 'operator'),
    (5, 'alerts'),
    (5, 'disruption')
;

insert
into
    deads_test_to_feature  (test_id,
                            feature_id)
select
    t.id,
    6
from
    tests t
where
    (t."name" like '%api-machinery%'
        or t."name" like '%apimachinery%'
        or t."name" like '%kube-apiserver%')
  and t."name" like '%operator%';



select
    *
from
    tests t
where
    (t."name" like '%api-machinery%'
        or t."name" like '%apimachinery%'
        or t."name" like '%kube-apiserver%')
  and t."name" like '%operator%';