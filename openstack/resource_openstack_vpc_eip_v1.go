package openstack

import (
	"fmt"
	"log"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/vpc/v1/bandwidths"
	"github.com/gophercloud/gophercloud/openstack/vpc/v1/eips"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceVpcEIPV1() *schema.Resource {
	return &schema.Resource{
		Create: resourceVpcEIPV1Create,
		Read:   resourceVpcEIPV1Read,
		Update: resourceVpcEIPV1Update,
		Delete: resourceVpcEIPV1Delete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(10 * time.Minute),
			Delete: schema.DefaultTimeout(10 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"publicip": &schema.Schema{
				Type:     schema.TypeList,
				Required: true,
				ForceNew: false,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"type": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
						"ip_address": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
						},
						"port_id": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: false,
						},
					},
				},
			},
			"bandwidth": &schema.Schema{
				Type:     schema.TypeList,
				Required: true,
				ForceNew: false,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"name": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
							ForceNew: false,
						},
						"size": &schema.Schema{
							Type:     schema.TypeInt,
							Required: true,
							ForceNew: false,
						},
						"share_type": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
							ForceNew: true,
						},
						"charge_mode": &schema.Schema{
							Type:     schema.TypeString,
							Optional: true,
							ForceNew: true,
						},
					},
				},
			},
			"value_specs": &schema.Schema{
				Type:     schema.TypeMap,
				Optional: true,
				ForceNew: false,
			},
		},
	}
}

func resourceVpcEIPV1Create(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	vpcClient, err := config.vpcV1Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating VPC client: %s", err)
	}

	createOpts := EIPCreateOpts{
		eips.ApplyOpts{
			IP:        resourcePublicIP(d),
			Bandwidth: resourceBandWidth(d),
		},
		MapValueSpecs(d),
	}

	log.Printf("[DEBUG] Create Options: %#v", createOpts)
	eIP, err := eips.Apply(vpcClient, createOpts).Extract()
	if err != nil {
		return fmt.Errorf("Error allocating EIP: %s", err)
	}

	log.Printf("[DEBUG] Waiting for EIP %s to become available.", eIP)

	stateConf := &resource.StateChangeConf{
		Target:     []string{"ACTIVE"},
		Refresh:    waitForEIPActive(vpcClient, eIP.ID),
		Timeout:    d.Timeout(schema.TimeoutCreate),
		Delay:      5 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err = stateConf.WaitForState()

	d.SetId(eIP.ID)

	return resourceVpcEIPV1Read(d, meta)
}

func resourceVpcEIPV1Read(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	vpcClient, err := config.vpcV1Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating VPC client: %s", err)
	}

	eIP, err := eips.Get(vpcClient, d.Id()).Extract()
	if err != nil {
		return CheckDeleted(d, err, "eIP")
	}
	bandWidth, err := bandwidths.Get(vpcClient, eIP.BandwidthID).Extract()
	if err != nil {
		return fmt.Errorf("Error fetching bandwidth: %s", err)
	}

	// Set public ip
	publicIP := []map[string]string{
		{
			"type":       eIP.Type,
			"ip_address": eIP.PublicAddress,
			"port_id":    eIP.PortID,
		},
	}
	d.Set("publicip", publicIP)

	// Set bandwidth
	bW := []map[string]interface{}{
		{
			"name":        bandWidth.Name,
			"size":        eIP.BandwidthSize,
			"share_type":  eIP.BandwidthShareType,
			"charge_mode": bandWidth.ChargeMode,
		},
	}
	d.Set("bandwidth", bW)

	return nil
}

func resourceVpcEIPV1Update(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	vpcClient, err := config.vpcV1Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating EIP client: %s", err)
	}

	// Update bandwidth change
	if d.HasChange("bandwidth") {
		var updateOpts bandwidths.UpdateOpts

		newBWList := d.Get("bandwidth").([]interface{})
		newMap := newBWList[0].(map[string]interface{})
		updateOpts.Size = newMap["size"].(int)
		updateOpts.Name = newMap["name"].(string)

		log.Printf("[DEBUG] Bandwidth Update Options: %#v", updateOpts)

		eIP, err := eips.Get(vpcClient, d.Id()).Extract()
		if err != nil {
			return CheckDeleted(d, err, "eIP")
		}
		_, err = bandwidths.Update(vpcClient, eIP.BandwidthID, updateOpts).Extract()
		if err != nil {
			return fmt.Errorf("Error updating bandwidth: %s", err)
		}

	}

	// Update publicip change
	if d.HasChange("publicip") {
		var updateOpts eips.UpdateOpts

		newIPList := d.Get("publicip").([]interface{})
		newMap := newIPList[0].(map[string]interface{})
		updateOpts.PortID = newMap["port_id"].(string)

		log.Printf("[DEBUG] PublicIP Update Options: %#v", updateOpts)
		_, err = eips.Update(vpcClient, d.Id(), updateOpts).Extract()
		if err != nil {
			return fmt.Errorf("Error updating publicip: %s", err)
		}

	}

	return resourceVpcEIPV1Read(d, meta)
}

func resourceVpcEIPV1Delete(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*Config)
	vpcClient, err := config.vpcV1Client(GetRegion(d, config))
	if err != nil {
		return fmt.Errorf("Error creating VPC client: %s", err)
	}

	stateConf := &resource.StateChangeConf{
		Pending:    []string{"ACTIVE"},
		Target:     []string{"DELETED"},
		Refresh:    waitForEIPDelete(vpcClient, d.Id()),
		Timeout:    d.Timeout(schema.TimeoutCreate),
		Delay:      5 * time.Second,
		MinTimeout: 3 * time.Second,
	}

	_, err = stateConf.WaitForState()
	if err != nil {
		return fmt.Errorf("Error deleting EIP: %s", err)
	}

	d.SetId("")

	return nil
}

func waitForEIPActive(vpcClient *gophercloud.ServiceClient, eId string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		e, err := eips.Get(vpcClient, eId).Extract()
		if err != nil {
			return nil, "", err
		}

		log.Printf("[DEBUG] EIP: %+v", e)
		if e.Status == "DOWN" || e.Status == "ACTIVE" {
			return e, "ACTIVE", nil
		}

		return e, "", nil
	}
}

func waitForEIPDelete(vpcClient *gophercloud.ServiceClient, eId string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		log.Printf("[DEBUG] Attempting to delete EIP %s.\n", eId)

		e, err := eips.Get(vpcClient, eId).Extract()
		if err != nil {
			if _, ok := err.(gophercloud.ErrDefault404); ok {
				log.Printf("[DEBUG] Successfully deleted EIP %s", eId)
				return e, "DELETED", nil
			}
			return e, "ACTIVE", err
		}

		err = eips.Delete(vpcClient, eId).ExtractErr()
		if err != nil {
			if _, ok := err.(gophercloud.ErrDefault404); ok {
				log.Printf("[DEBUG] Successfully deleted EIP %s", eId)
				return e, "DELETED", nil
			}
			return e, "ACTIVE", err
		}

		log.Printf("[DEBUG] EIP %s still active.\n", eId)
		return e, "ACTIVE", nil
	}
}

func resourcePublicIP(d *schema.ResourceData) eips.PublicIpOpts {
	publicIPRaw := d.Get("publicip").([]interface{})
	rawMap := publicIPRaw[0].(map[string]interface{})

	publicip := eips.PublicIpOpts{
		Type:    rawMap["type"].(string),
		Address: rawMap["ip_address"].(string),
	}
	return publicip
}

func resourceBandWidth(d *schema.ResourceData) eips.BandwidthOpts {
	bandwidthRaw := d.Get("bandwidth").([]interface{})
	rawMap := bandwidthRaw[0].(map[string]interface{})

	bandwidth := eips.BandwidthOpts{
		Name:       rawMap["name"].(string),
		Size:       rawMap["size"].(int),
		ShareType:  rawMap["share_type"].(string),
		ChargeMode: rawMap["charge_mode"].(string),
	}
	return bandwidth
}
