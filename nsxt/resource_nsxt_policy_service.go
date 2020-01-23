/* Copyright © 2019 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: MPL-2.0 */

package nsxt

import (
	"fmt"
	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/helper/validation"
	"github.com/vmware/vsphere-automation-sdk-go/runtime/bindings"
	"github.com/vmware/vsphere-automation-sdk-go/runtime/data"
	"github.com/vmware/vsphere-automation-sdk-go/runtime/protocol/client"
	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt/infra"
	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt/model"
	"log"
	"strconv"
)

func resourceNsxtPolicyService() *schema.Resource {
	return &schema.Resource{
		Create: resourceNsxtPolicyServiceCreate,
		Read:   resourceNsxtPolicyServiceRead,
		Update: resourceNsxtPolicyServiceUpdate,
		Delete: resourceNsxtPolicyServiceDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"nsx_id":       getNsxIDSchema(),
			"path":         getPathSchema(),
			"display_name": getDisplayNameSchema(),
			"description":  getDescriptionSchema(),
			"revision":     getRevisionSchema(),
			"tag":          getTagsSchema(),

			"icmp_entry": {
				Type:        schema.TypeSet,
				Description: "ICMP type service entry",
				Optional:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"display_name": getOptionalDisplayNameSchema(),
						"description":  getDescriptionSchema(),
						"protocol": {
							Type:         schema.TypeString,
							Description:  "Version of ICMP protocol (ICMPv4/ICMPv6)",
							Required:     true,
							ValidateFunc: validation.StringInSlice(icmpProtocolValues, false),
						},
						"icmp_type": {
							// NOTE: icmp_type is required if icmp_code is set
							Type:         schema.TypeString,
							Description:  "ICMP message type",
							Optional:     true,
							ValidateFunc: validateStringIntBetween(0, 255),
						},
						"icmp_code": {
							Type:         schema.TypeString,
							Description:  "ICMP message code",
							Optional:     true,
							ValidateFunc: validateStringIntBetween(0, 255),
						},
					},
				},
			},

			"l4_port_set_entry": {
				Type:        schema.TypeSet,
				Description: "L4 port set type service entry",
				Optional:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"display_name": getOptionalDisplayNameSchema(),
						"description":  getDescriptionSchema(),
						"destination_ports": {
							Type:        schema.TypeSet,
							Description: "Set of destination ports",
							Elem: &schema.Schema{
								Type:         schema.TypeString,
								ValidateFunc: validatePortRange(),
							},
							Optional: true,
						},
						"source_ports": {
							Type:        schema.TypeSet,
							Description: "Set of source ports",
							Elem: &schema.Schema{
								Type:         schema.TypeString,
								ValidateFunc: validatePortRange(),
							},
							Optional: true,
						},
						"protocol": {
							Type:         schema.TypeString,
							Description:  "L4 Protocol",
							Required:     true,
							ValidateFunc: validation.StringInSlice(protocolValues, false),
						},
					},
				},
			},

			"igmp_entry": {
				Type:        schema.TypeSet,
				Description: "IGMP type service entry",
				Optional:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"display_name": getOptionalDisplayNameSchema(),
						"description":  getDescriptionSchema(),
					},
				},
			},

			"ether_type_entry": {
				Type:        schema.TypeSet,
				Description: "Ether type service entry",
				Optional:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"display_name": getOptionalDisplayNameSchema(),
						"description":  getDescriptionSchema(),
						"ether_type": {
							Type:        schema.TypeInt,
							Description: "Type of the encapsulated protocol",
							Required:    true,
						},
					},
				},
			},

			"ip_protocol_entry": {
				Type:        schema.TypeSet,
				Description: "IP Protocol type service entry",
				Optional:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"display_name": getOptionalDisplayNameSchema(),
						"description":  getDescriptionSchema(),
						"protocol": {
							Type:         schema.TypeInt,
							Description:  "IP protocol number",
							Required:     true,
							ValidateFunc: validation.IntBetween(0, 255),
						},
					},
				},
			},

			"algorithm_entry": {
				Type:        schema.TypeSet,
				Description: "Algorithm type service entry",
				Optional:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"display_name": getOptionalDisplayNameSchema(),
						"description":  getDescriptionSchema(),
						"destination_port": {
							Type:         schema.TypeString,
							Description:  "A single destination port",
							Required:     true,
							ValidateFunc: validateSinglePort(),
						},
						"source_ports": {
							Type:        schema.TypeSet,
							Description: "Set of source ports or ranges",
							Elem: &schema.Schema{
								Type:         schema.TypeString,
								ValidateFunc: validatePortRange(),
							},
							Optional: true,
						},
						"algorithm": {
							Type:         schema.TypeString,
							Description:  "Algorithm",
							Required:     true,
							ValidateFunc: validation.StringInSlice(algTypeValues, false),
						},
					},
				},
			},
		},
	}
}

func resourceNsxtPolicyServiceGetEntriesFromSchema(d *schema.ResourceData) ([]*data.StructValue, error) {
	converter := bindings.NewTypeConverter()
	converter.SetMode(bindings.REST)
	serviceEntries := []*data.StructValue{}

	// ICMP Type service entries
	icmpEntries := d.Get("icmp_entry").(*schema.Set).List()
	for _, icmpEntry := range icmpEntries {
		entryData := icmpEntry.(map[string]interface{})
		// Type and code can be unset
		var typePtr *int64
		var codePtr *int64
		if entryData["icmp_type"] != "" {
			icmpType, err := strconv.Atoi(entryData["icmp_type"].(string))
			if err != nil {
				return serviceEntries, err
			}
			icmpType64 := int64(icmpType)
			typePtr = &icmpType64
		}
		if entryData["icmp_code"] != "" {
			icmpCode, err := strconv.Atoi(entryData["icmp_code"].(string))
			if err != nil {
				return serviceEntries, err
			}
			icmpCode64 := int64(icmpCode)
			codePtr = &icmpCode64
		}
		protocol := entryData["protocol"].(string)
		displayName := entryData["display_name"].(string)
		description := entryData["description"].(string)

		// Use a different random Id each time
		id := newUUID()

		serviceEntry := model.ICMPTypeServiceEntry{
			Id:           &id,
			DisplayName:  &displayName,
			Description:  &description,
			IcmpType:     typePtr,
			IcmpCode:     codePtr,
			Protocol:     protocol,
			ResourceType: model.ServiceEntry_RESOURCE_TYPE_ICMPTYPESERVICEENTRY,
		}
		dataValue, errs := converter.ConvertToVapi(serviceEntry, model.ICMPTypeServiceEntryBindingType())
		if errs != nil {
			return serviceEntries, errs[0]
		}
		var entryStruct *data.StructValue
		entryStruct = dataValue.(*data.StructValue)
		serviceEntries = append(serviceEntries, entryStruct)
	}

	// L4 port set Type service entries
	l4Entries := d.Get("l4_port_set_entry").(*schema.Set).List()
	for _, l4Entry := range l4Entries {
		entryData := l4Entry.(map[string]interface{})
		l4Protocol := entryData["protocol"].(string)
		sourcePorts := interface2StringList(entryData["source_ports"].(*schema.Set).List())
		destinationPorts := interface2StringList(entryData["destination_ports"].(*schema.Set).List())
		displayName := entryData["display_name"].(string)
		description := entryData["description"].(string)

		// Use a different random Id each time
		id := newUUID()

		serviceEntry := model.L4PortSetServiceEntry{
			Id:               &id,
			DisplayName:      &displayName,
			Description:      &description,
			DestinationPorts: destinationPorts,
			SourcePorts:      sourcePorts,
			L4Protocol:       l4Protocol,
			ResourceType:     model.ServiceEntry_RESOURCE_TYPE_L4PORTSETSERVICEENTRY,
		}
		dataValue, errs := converter.ConvertToVapi(serviceEntry, model.L4PortSetServiceEntryBindingType())
		if errs != nil {
			return serviceEntries, errs[0]
		}
		var entryStruct *data.StructValue
		entryStruct = dataValue.(*data.StructValue)
		serviceEntries = append(serviceEntries, entryStruct)
	}

	// IGMP Type service entries
	igmpEntries := d.Get("igmp_entry").(*schema.Set).List()
	for _, igmpEntry := range igmpEntries {
		entryData := igmpEntry.(map[string]interface{})
		displayName := entryData["display_name"].(string)
		description := entryData["description"].(string)

		// Use a different random Id each time
		id := newUUID()

		serviceEntry := model.IGMPTypeServiceEntry{
			Id:           &id,
			DisplayName:  &displayName,
			Description:  &description,
			ResourceType: model.ServiceEntry_RESOURCE_TYPE_IGMPTYPESERVICEENTRY,
		}
		dataValue, errs := converter.ConvertToVapi(serviceEntry, model.IGMPTypeServiceEntryBindingType())
		if errs != nil {
			return serviceEntries, errs[0]
		}
		var entryStruct *data.StructValue
		entryStruct = dataValue.(*data.StructValue)
		serviceEntries = append(serviceEntries, entryStruct)
	}

	// Ether Type service entries
	etherEntries := d.Get("ether_type_entry").(*schema.Set).List()
	for _, etherEntry := range etherEntries {
		entryData := etherEntry.(map[string]interface{})
		displayName := entryData["display_name"].(string)
		description := entryData["description"].(string)
		etherType := int64(entryData["ether_type"].(int))

		// Use a different random Id each time
		id := newUUID()

		serviceEntry := model.EtherTypeServiceEntry{
			Id:           &id,
			DisplayName:  &displayName,
			Description:  &description,
			EtherType:    etherType,
			ResourceType: model.ServiceEntry_RESOURCE_TYPE_ETHERTYPESERVICEENTRY,
		}
		dataValue, errs := converter.ConvertToVapi(serviceEntry, model.EtherTypeServiceEntryBindingType())
		if errs != nil {
			return serviceEntries, errs[0]
		}
		var entryStruct *data.StructValue
		entryStruct = dataValue.(*data.StructValue)
		serviceEntries = append(serviceEntries, entryStruct)
	}

	// IP Protocol Type service entries
	ipProtEntries := d.Get("ip_protocol_entry").(*schema.Set).List()
	for _, ipProtEntry := range ipProtEntries {
		entryData := ipProtEntry.(map[string]interface{})
		displayName := entryData["display_name"].(string)
		description := entryData["description"].(string)
		protocolNumber := int64(entryData["protocol"].(int))

		// Use a different random Id each time
		id := newUUID()

		serviceEntry := model.IPProtocolServiceEntry{
			Id:             &id,
			DisplayName:    &displayName,
			Description:    &description,
			ProtocolNumber: protocolNumber,
			ResourceType:   model.ServiceEntry_RESOURCE_TYPE_IPPROTOCOLSERVICEENTRY,
		}
		dataValue, errs := converter.ConvertToVapi(serviceEntry, model.IPProtocolServiceEntryBindingType())
		if errs != nil {
			return serviceEntries, errs[0]
		}
		var entryStruct *data.StructValue
		entryStruct = dataValue.(*data.StructValue)
		serviceEntries = append(serviceEntries, entryStruct)
	}

	// Algorithm Type service entries
	algEntries := d.Get("algorithm_entry").(*schema.Set).List()
	for _, algEntry := range algEntries {
		entryData := algEntry.(map[string]interface{})
		displayName := entryData["display_name"].(string)
		description := entryData["description"].(string)
		alg := entryData["algorithm"].(string)
		sourcePorts := interface2StringList(entryData["source_ports"].(*schema.Set).List())
		destinationPorts := make([]string, 0, 1)
		destinationPorts = append(destinationPorts, entryData["destination_port"].(string))

		// Use a different random Id each time
		id := newUUID()

		serviceEntry := model.ALGTypeServiceEntry{
			Id:               &id,
			DisplayName:      &displayName,
			Description:      &description,
			Alg:              alg,
			DestinationPorts: destinationPorts,
			SourcePorts:      sourcePorts,
			ResourceType:     model.ServiceEntry_RESOURCE_TYPE_ALGTYPESERVICEENTRY,
		}
		dataValue, errs := converter.ConvertToVapi(serviceEntry, model.ALGTypeServiceEntryBindingType())
		if errs != nil {
			return serviceEntries, errs[0]
		}
		var entryStruct *data.StructValue
		entryStruct = dataValue.(*data.StructValue)
		serviceEntries = append(serviceEntries, entryStruct)
	}

	return serviceEntries, nil
}

func resourceNsxtPolicyServiceExists(id string, connector *client.RestConnector) bool {
	client := infra.NewDefaultServicesClient(connector)

	_, err := client.Get(id)
	if err == nil {
		return true
	}

	if isNotFoundError(err) {
		return false
	}

	logAPIError("Error retrieving service", err)
	return false
}

func filterServiceEntryDisplayName(entryDisplayName string, entryID string) string {
	if entryDisplayName == entryID {
		return ""
	}
	return entryDisplayName
}

func resourceNsxtPolicyServiceCreate(d *schema.ResourceData, m interface{}) error {
	connector := getPolicyConnector(m)
	client := infra.NewDefaultServicesClient(connector)

	// Initialize resource Id and verify this ID is not yet used
	id, err := getOrGenerateID(d, connector, resourceNsxtPolicyServiceExists)
	if err != nil {
		return err
	}

	displayName := d.Get("display_name").(string)
	description := d.Get("description").(string)
	tags := getPolicyTagsFromSchema(d)
	serviceEntries, errc := resourceNsxtPolicyServiceGetEntriesFromSchema(d)
	if errc != nil {
		return fmt.Errorf("Error during Service entries conversion: %v", errc)
	}

	obj := model.Service{
		DisplayName:    &displayName,
		Description:    &description,
		Tags:           tags,
		ServiceEntries: serviceEntries,
	}

	// Create the resource using PATCH
	log.Printf("[INFO] Creating service with ID %s", id)
	err = client.Patch(id, obj)
	if err != nil {
		return handleCreateError("Service", id, err)
	}

	d.SetId(id)
	d.Set("nsx_id", id)
	return resourceNsxtPolicyServiceRead(d, m)
}

func resourceNsxtPolicyServiceRead(d *schema.ResourceData, m interface{}) error {
	connector := getPolicyConnector(m)
	client := infra.NewDefaultServicesClient(connector)

	id := d.Id()
	if id == "" {
		return fmt.Errorf("Error obtaining service id")
	}

	obj, err := client.Get(id)
	if err != nil {
		return handleReadError(d, "Service", id, err)
	}

	d.Set("display_name", obj.DisplayName)
	d.Set("description", obj.Description)
	setPolicyTagsInSchema(d, obj.Tags)
	d.Set("nsx_id", id)
	d.Set("path", obj.Path)
	d.Set("revision", obj.Revision)

	// Translate the returned service entries
	converter := bindings.NewTypeConverter()
	converter.SetMode(bindings.REST)
	var icmpEntriesList []map[string]interface{}
	var l4EntriesList []map[string]interface{}
	var igmpEntriesList []map[string]interface{}
	var etherEntriesList []map[string]interface{}
	var ipProtEntriesList []map[string]interface{}
	var algEntriesList []map[string]interface{}

	for _, entry := range obj.ServiceEntries {
		elem := make(map[string]interface{})
		icmpEntry, errs := converter.ConvertToGolang(entry, model.ICMPTypeServiceEntryBindingType())
		if errs == nil {
			serviceEntry := icmpEntry.(model.ICMPTypeServiceEntry)
			elem["display_name"] = filterServiceEntryDisplayName(*serviceEntry.DisplayName, *serviceEntry.Id)
			elem["description"] = serviceEntry.Description
			if serviceEntry.IcmpType != nil {
				elem["icmp_type"] = strconv.Itoa(int(*serviceEntry.IcmpType))
			} else {
				elem["icmp_type"] = ""
			}
			if serviceEntry.IcmpCode != nil {
				elem["icmp_code"] = strconv.Itoa(int(*serviceEntry.IcmpCode))
			} else {
				elem["icmp_code"] = ""
			}
			elem["protocol"] = serviceEntry.Protocol
			icmpEntriesList = append(icmpEntriesList, elem)
		} else {
			l4Entry, l4Errs := converter.ConvertToGolang(entry, model.L4PortSetServiceEntryBindingType())
			if l4Errs == nil {
				serviceEntry := l4Entry.(model.L4PortSetServiceEntry)
				elem["display_name"] = filterServiceEntryDisplayName(*serviceEntry.DisplayName, *serviceEntry.Id)
				elem["description"] = serviceEntry.Description
				elem["destination_ports"] = serviceEntry.DestinationPorts
				elem["source_ports"] = serviceEntry.SourcePorts
				elem["protocol"] = serviceEntry.L4Protocol
				l4EntriesList = append(l4EntriesList, elem)
			} else {
				etherEntry, etherErrs := converter.ConvertToGolang(entry, model.EtherTypeServiceEntryBindingType())
				if etherErrs == nil {
					serviceEntry := etherEntry.(model.EtherTypeServiceEntry)
					elem["display_name"] = filterServiceEntryDisplayName(*serviceEntry.DisplayName, *serviceEntry.Id)
					elem["description"] = serviceEntry.Description
					elem["ether_type"] = serviceEntry.EtherType
					etherEntriesList = append(etherEntriesList, elem)
				} else {
					ipProtEntry, ipProtErrs := converter.ConvertToGolang(entry, model.IPProtocolServiceEntryBindingType())
					if ipProtErrs == nil {
						serviceEntry := ipProtEntry.(model.IPProtocolServiceEntry)
						elem["display_name"] = filterServiceEntryDisplayName(*serviceEntry.DisplayName, *serviceEntry.Id)
						elem["description"] = serviceEntry.Description
						elem["protocol"] = serviceEntry.ProtocolNumber
						ipProtEntriesList = append(ipProtEntriesList, elem)
					} else {
						algEntry, algErrs := converter.ConvertToGolang(entry, model.ALGTypeServiceEntryBindingType())
						if algErrs == nil {
							serviceEntry := algEntry.(model.ALGTypeServiceEntry)
							elem["display_name"] = filterServiceEntryDisplayName(*serviceEntry.DisplayName, *serviceEntry.Id)
							elem["description"] = serviceEntry.Description
							elem["algorithm"] = serviceEntry.Alg
							elem["destination_port"] = serviceEntry.DestinationPorts[0]
							elem["source_ports"] = serviceEntry.SourcePorts
							algEntriesList = append(algEntriesList, elem)
						} else {
							igmpEntry, igmpErrs := converter.ConvertToGolang(entry, model.IGMPTypeServiceEntryBindingType())
							if igmpErrs == nil {
								serviceEntry := igmpEntry.(model.IGMPTypeServiceEntry)
								elem["display_name"] = filterServiceEntryDisplayName(*serviceEntry.DisplayName, *serviceEntry.Id)
								elem["description"] = serviceEntry.Description
								igmpEntriesList = append(igmpEntriesList, elem)
							} else {
								// Unknown service entry type
								return igmpErrs[0]
							}
						}
					}
				}
			}
		}
	}

	err = d.Set("icmp_entry", icmpEntriesList)
	if err != nil {
		return err
	}

	err = d.Set("l4_port_set_entry", l4EntriesList)
	if err != nil {
		return err
	}

	err = d.Set("igmp_entry", igmpEntriesList)
	if err != nil {
		return err
	}

	err = d.Set("ether_type_entry", etherEntriesList)
	if err != nil {
		return err
	}

	err = d.Set("ip_protocol_entry", ipProtEntriesList)
	if err != nil {
		return err
	}

	err = d.Set("algorithm_entry", algEntriesList)
	if err != nil {
		return err
	}

	return nil
}

func resourceNsxtPolicyServiceUpdate(d *schema.ResourceData, m interface{}) error {
	connector := getPolicyConnector(m)
	client := infra.NewDefaultServicesClient(connector)

	id := d.Id()
	if id == "" {
		return fmt.Errorf("Error obtaining service id")
	}

	// Read the rest of the configured parameters
	displayName := d.Get("display_name").(string)
	description := d.Get("description").(string)
	revision := int64(d.Get("revision").(int))
	tags := getPolicyTagsFromSchema(d)
	serviceEntries, errc := resourceNsxtPolicyServiceGetEntriesFromSchema(d)
	if errc != nil {
		return fmt.Errorf("Error during Service entries conversion: %v", errc)
	}
	obj := model.Service{
		DisplayName:    &displayName,
		Description:    &description,
		Tags:           tags,
		ServiceEntries: serviceEntries,
		Revision:       &revision,
	}

	// Update the resource using Update to totally replace the list of entries
	_, err := client.Update(id, obj)
	if err != nil {
		return handleUpdateError("Service", id, err)
	}
	return resourceNsxtPolicyServiceRead(d, m)
}

func resourceNsxtPolicyServiceDelete(d *schema.ResourceData, m interface{}) error {
	id := d.Id()
	if id == "" {
		return fmt.Errorf("Error obtaining service id")
	}

	connector := getPolicyConnector(m)
	client := infra.NewDefaultServicesClient(connector)
	err := client.Delete(id)
	if err != nil {
		err = handleDeleteError("Service", id, err)
	}

	return nil
}